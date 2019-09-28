package main

import (
	"encoding/json"
	"fmt"
	"golang.org/x/net/http2"
	"log"
	"net/http"
	"os"
	"path"
)

type LaunchResponse struct {
	Status string `json:"status"`
}

func NewLaunchResponse(status string) *LaunchResponse {
	return &LaunchResponse{Status: status}
}

const ContentType = "Content-Type"
const ApplicationJson = "application/json; charset=utf-8"

func GetRequestOrigin(request *http.Request) string {
	return (*request).Header.Get("Origin")
}

func WriteJson(body interface{}) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set(ContentType, ApplicationJson)
		body, err := json.Marshal(body)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, err = writer.Write(body)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
		}

	}
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
	"https://localhost:8443",
	"https://mars.local:3443",
	EnvOrDefault("FRONTEND_ORIGIN", "https://mars.local:8443"),
}

var initial = WriteJson(NewLaunchResponse("N/A"))

var launch = func() http.HandlerFunc {
	return WriteJson(NewLaunchResponse("Pending"))
}

var terminate = WriteJson(NewLaunchResponse("Terminated"))

func main() {
	http.HandleFunc("/status", Get(initial))
	http.HandleFunc("/launch", Post(launch()))
	http.HandleFunc("/terminate", Post(terminate))

	var err error

	host := EnvOrDefault("HOST", "0.0.0.0")
	port := EnvOrDefault("PORT", "9443")

	server := http.Server{
		Addr:    fmt.Sprintf("%s:%s", host, port),
		Handler: nil,
	}
	conf2 := http2.Server{}

	if err = http2.ConfigureServer(&server, &conf2); err != nil {
		log.Fatalf("HTTP2 error %s", err)
	}

	certDir := EnvOrDefault("CERT_DIR", "/home/john/algo/wpr/certs")
	certName := EnvOrDefault("CERT_NAME", "selfsigned")
	cert := path.Join(certDir, certName)

	log.Fatal(server.ListenAndServeTLS(cert+".crt", cert+".key"), nil)
}

func EnvOrDefault(key, def string) string {
	if val, ok := os.LookupEnv(key); !ok {
		return def
	} else {
		return val
	}
}
