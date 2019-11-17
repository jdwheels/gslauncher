package main

import (
	"context"
	"defilade.io/gslauncher/pkg/sse"
	status2 "defilade.io/gslauncher/pkg/status"
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
	writeJson(&writer, status2.ExtendedLaunchResponse{
		Status:       "N/A",
		IsLaunched:   isLaunched,
		IsTerminated: isTerminated,
		Date:         time.Now().Unix(),
	})
}

var isTerminated = true
var isLaunched = false
var clusterName = EnvOrDefault("CLUSTER_NAME", "EC2ContainerService-game-servers-2-EcsInstanceAsg-9AB2NHDSISGL")
var isProd = EnvOrDefault("GOENV", "dev") == "production"

func awsAction(writer *http.ResponseWriter, action func(string) bool, status string, toggle func()) {
	if success := actionWrapper(clusterName, action); success {
		toggle()
		writeJson(writer, status2.NewLaunchResponse(status))
	} else {
		(*writer).WriteHeader(http.StatusInternalServerError)
	}
}

func actionWrapper(target string, action func(string) bool) bool {
	log.Printf("GOENV => %s => isProd => %t", EnvOrDefault("GOENV", "dev"), isProd)
	if isProd {
		return action(target)
	}
	return dryAction(target)
}

func dryAction(target string) bool {
	log.Printf("Simulating action on '%s'", target)
	return true
}

func awsEvent(writer *http.ResponseWriter, status string, toggle func()) {
	toggle()
	writeJson(writer, status2.NewLaunchResponse(status))
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

func launched(broker *sse.Broker) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		broker.SimpleEvent("launched")
		awsEvent(&writer, "Ok", func() {
			isTerminated = false
			isLaunched = true
		})
	}
}

func terminate(writer http.ResponseWriter, _ *http.Request) {
	awsAction(&writer, x.StopEC2Cluster, "Terminating", func() {
		isTerminated = false
		isLaunched = false
	})
}

func terminated(broker *sse.Broker) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		broker.SimpleEvent("terminated")
		awsEvent(&writer, "Ok", func() {
			isLaunched = false
			isTerminated = true
		})
	}
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

func configureHttp2Server(server *http.Server) (f func() error, err error) {
	conf2 := http2.Server{}

	if err = http2.ConfigureServer(server, &conf2); err != nil {
		log.Fatalf("HTTP2 error %s", err)
	}

	certDir := EnvOrDefault("CERT_DIR", "/home/john/algo/wpr/certs")
	certName := EnvOrDefault("CERT_NAME", "selfsigned")
	cert := path.Join(certDir, certName)
	f = func() error {
		return server.ListenAndServeTLS(cert+".crt", cert+".key")
	}
	return
}

func configureServer(mux *http.ServeMux, host, port string) (server *http.Server, f func() error) {
	server = newServer(mux, host, port)

	useHttp2, _ := strconv.ParseBool(EnvOrDefault("USE_HTTP2", "true"))

	if useHttp2 {
		var err error
		f, err = configureHttp2Server(server)
		if err != nil {
			log.Fatalf("Configure error: %v", err)
		}
	} else {
		f = func() error {
			return server.ListenAndServe()
		}
	}
	return
}

func HandleClose(idleConnsClosed chan struct{}, servers []*http.Server) {
	sigint := make(chan os.Signal, 1)

	signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)

	<-sigint

	// We received an interrupt signal, shut down.
	for _, server := range servers {
		if err := server.Shutdown(context.Background()); err != nil {
			// Error from closing listeners, or context timeout:
			log.Printf("HTTP server Shutdown: %v", err)
		} else {
			log.Printf("Shutting down...")
		}
	}

	close(idleConnsClosed)
}

func newServer(mux *http.ServeMux, host, port string) *http.Server {
	return &http.Server{
		Addr:    fmt.Sprintf("%s:%s", host, port),
		Handler: logRequest(mux),
	}
}


func main() {
	idleConnsClosed := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/status", Get(initial))
	mux.HandleFunc("/status-x", Get(initial))
	mux.HandleFunc("/launch", Post(launch))
	mux.HandleFunc("/terminate", Post(terminate))

	mux.HandleFunc("/err", Get(errTest))
	host := EnvOrDefault("HOST", "0.0.0.0")
	port := EnvOrDefault("PORT", "9443")
	server, fS := configureServer(mux, host, port)

	broker := sse.NewBroker()


	muxE := http.NewServeMux()
	muxE.Handle("/listen", broker)
	muxE.HandleFunc("/event", broker.Event)
	muxE.HandleFunc("/terminated", Post(terminated(broker)))
	muxE.HandleFunc("/launched", Post(launched(broker)))
	portE := EnvOrDefault("PORT_E", "9444")
	serverE, fE := configureServer(muxE, host, portE)

	servers := []*http.Server{server, serverE}

	go HandleClose(idleConnsClosed, servers)

	fs := []*func() error{&fS, &fE}
	log.Print(len(fs))
	for i := 0; i < len(fs); i++  {
		log.Print(i)
		f := fs[i]
		go func() {
			err := (*f)()
			if err != nil && err != http.ErrServerClosed {
				// Error starting or closing listener:
				log.Fatalf("HTTP server ListenAndServe: %v", err)
			}
		}()
	}

	<-idleConnsClosed
	log.Printf("Shutdown.")
}
