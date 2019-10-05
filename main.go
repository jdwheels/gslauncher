package main

import (
	"context"
	"encoding/json"
	"fmt"
	x "github.com/jdwheels/xaws/pkg/ec2"
	"golang.org/x/net/http2"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"syscall"
	"time"
)

func EnvOrDefault(key, def string) string {
	if val, ok := os.LookupEnv(key); !ok {
		return def
	} else {
		return val
	}
}

type LaunchResponse struct {
	Status string `json:"status"`
}

type ExtendedLaunchResponse struct {
	Status       string `json:"status"`
	IsLaunched   bool   `json:"is_launched"`
	IsTerminated bool   `json:"is_terminated"`
	Date         int64  `json:"date"`
}

func NewLaunchResponse(status string) *LaunchResponse {
	return &LaunchResponse{Status: status}
}

const ContentType = "Content-Type"
const ApplicationJson = "application/json"

func GetRequestOrigin(request *http.Request) string {
	return (*request).Header.Get("Origin")
}

func setupResponse(w *http.ResponseWriter, req *http.Request) {
	for _, allowedOrigin := range *AllowedOrigins {
		if allowedOrigin == GetRequestOrigin(req) {
			(*w).Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			(*w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		}
	}
}

func Method(method string, handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		setupResponse(&writer, request)
		switch request.Method {
		case http.MethodOptions:
		case method:
			handlerFunc(writer, request)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func Get(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return Method(http.MethodGet, handlerFunc)
}

func Post(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return Method(http.MethodPost, handlerFunc)
}

func Options(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return Method(http.MethodOptions, handlerFunc)
}

var AllowedOrigins = &[]string{
	"https://localhost:3443",
	EnvOrDefault("FRONTEND_ORIGIN_LOCAL", "https://localhost:8443"),
	"https://mars.local:3443",
	EnvOrDefault("FRONTEND_ORIGIN", "https://mars.local:8443"),
}

func initial(writer http.ResponseWriter, _ *http.Request) {
	writeJson(&writer, ExtendedLaunchResponse{
		"N/A",
		isLaunched,
		isTerminated,
		time.Now().Unix(),
	})
}

var isTerminated = true
var isLaunched = false
var clusterName = EnvOrDefault("CLUSTER_NAME", "EC2ContainerService-game-servers-2-EcsInstanceAsg-9AB2NHDSISGL")

func awsAction(writer *http.ResponseWriter, action func(string) bool, status string, toggle func()) {
	if success := action(clusterName); success {
		toggle()
		writeJson(writer, NewLaunchResponse(status))
	} else {
		(*writer).WriteHeader(http.StatusInternalServerError)
	}
}

func awsEvent(writer *http.ResponseWriter, status string, toggle func()) {
	toggle()
	writeJson(writer, NewLaunchResponse(status))
}

func writeJson(writer *http.ResponseWriter, body interface{}) {
	(*writer).Header().Set(ContentType, ApplicationJson)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		(*writer).WriteHeader(http.StatusInternalServerError)
	} else if _, err = (*writer).Write(jsonBody); err != nil {
		(*writer).WriteHeader(http.StatusInternalServerError)
	}
}

func launch(writer http.ResponseWriter, _ *http.Request) {
	awsAction(&writer, x.StartEC2Cluster, "Pending", func() {
		isLaunched = false
		isTerminated = false
	})
}

func launched(writer http.ResponseWriter, _ *http.Request) {
	awsEvent(&writer, "Ok", func() {
		isTerminated = false
		isLaunched = true
	})
}

func terminate(writer http.ResponseWriter, _ *http.Request) {
	awsAction(&writer, x.StopEC2Cluster, "Terminating", func() {
		isTerminated = false
		isLaunched = false
	})
}

func terminated(writer http.ResponseWriter, _ *http.Request) {
	awsEvent(&writer, "Ok", func() {
		isLaunched = false
		isTerminated = true
	})
}

func errTest(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusBadRequest)
}

func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s \"%s %s\" \"%s\"\n", r.RemoteAddr, r.Method, r.URL, r.UserAgent())
		handler.ServeHTTP(w, r)
	})
}

func main() {
	idleConnsClosed := make(chan struct{})
	http.HandleFunc("/status", Get(initial))
	http.HandleFunc("/status-x", Get(initial))
	http.HandleFunc("/launch", Post(launch))
	http.HandleFunc("/terminate", Post(terminate))
	http.HandleFunc("/launched", Post(launched))
	http.HandleFunc("/terminated", Post(terminated))
	http.HandleFunc("/err", Get(errTest))

	var err error

	host := EnvOrDefault("HOST", "0.0.0.0")
	port := EnvOrDefault("PORT", "9443")

	server := http.Server{
		Addr:    fmt.Sprintf("%s:%s", host, port),
		Handler: logRequest(http.DefaultServeMux),
	}

	go func() {
		sigint := make(chan os.Signal, 1)

		// interrupt signal sent from terminal
		signal.Notify(sigint, os.Interrupt)
		// sigterm signal sent from kubernetes
		signal.Notify(sigint, syscall.SIGTERM)

		<-sigint

		// We received an interrupt signal, shut down.
		if err := server.Shutdown(context.Background()); err != nil {
			// Error from closing listeners, or context timeout:
			log.Printf("HTTP server Shutdown: %v", err)
		}
		close(idleConnsClosed)
	}()

	useHttp2, _ := strconv.ParseBool(EnvOrDefault("USE_HTTP2", "true"))

	if useHttp2 {
		conf2 := http2.Server{}

		if err = http2.ConfigureServer(&server, &conf2); err != nil {
			log.Fatalf("HTTP2 error %s", err)
		}

		certDir := EnvOrDefault("CERT_DIR", "/home/john/algo/wpr/certs")
		certName := EnvOrDefault("CERT_NAME", "selfsigned")
		cert := path.Join(certDir, certName)
		err = server.ListenAndServeTLS(cert+".crt", cert+".key")
	} else {
		err = server.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		// Error starting or closing listener:
		log.Printf("HTTP server ListenAndServe: %v", err)
	}

	<-idleConnsClosed
}
