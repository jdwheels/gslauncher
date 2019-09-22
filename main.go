package main

import (
	"encoding/json"
	"golang.org/x/net/http2"
	"log"
	"net/http"
)

type LaunchResponse struct {
	Status string `json:"status"`
}

func NewLaunchResponse(status string) *LaunchResponse {
	return &LaunchResponse{Status: status}
}

const ContentType = "Content-Type"
const ApplicationJson = "application/json"

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
	"https://mars.local:8443",
}

var initial = WriteJson(NewLaunchResponse("N/A"))

var launch = func() http.HandlerFunc {
	return WriteJson(NewLaunchResponse("Pending"))
}

var terminate = WriteJson(NewLaunchResponse("Terminated"))

//func LoadX509KeyPair(certFile, keyFile string) (tls.Certificate, error) {
//	certPEMBlock, err := ioutil.ReadFile(certFile)
//	if err != nil {
//		return tls.Certificate{}, err
//	}
//	keyPEMBlock, err := ioutil.ReadFile(keyFile)
//	if err != nil {
//		return tls.Certificate{}, err
//	}
//	return tls.X509KeyPair(certPEMBlock, keyPEMBlock)
//}

func main() {
	http.HandleFunc("/status", Get(initial))
	http.HandleFunc("/launch", Post(launch()))
	http.HandleFunc("/terminate", Post(terminate))

	var err error
	//cert, err := LoadX509KeyPair(
	//	"/home/john/algo/wpr/certs/selfsigned.crt",
	//	"/home/john/algo/wpr/certs/selfsigned.key",
	//)
	//if err != nil {
	//	log.Fatalf("Cert error %s", err)
	//}

	//tlsConfig := &tls.Config{
	//	Certificates: []tls.Certificate{cert},
	//}
	server := http.Server{
		Addr:      "0.0.0.0:9443",
		Handler:   nil,
		//TLSConfig: tlsConfig,
	}
	conf2 := http2.Server{
		MaxHandlers:                  0,
		MaxConcurrentStreams:         0,
		MaxReadFrameSize:             0,
		PermitProhibitedCipherSuites: false,
		IdleTimeout:                  0,
		MaxUploadBufferPerConnection: 0,
		MaxUploadBufferPerStream:     0,
		NewWriteScheduler:            nil,
	}
	err = http2.ConfigureServer(&server, &conf2)

	if err != nil {
		log.Fatalf("HTTP2 error %s", err)
	}
	log.Fatal(server.ListenAndServeTLS(
		"/home/john/algo/wpr/certs/selfsigned.crt",
		"/home/john/algo/wpr/certs/selfsigned.key",
	), nil)
}
