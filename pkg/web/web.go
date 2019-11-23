package web

import (
	"defilade.io/gslauncher/pkg/utils"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func GetRequestOrigin(request *http.Request) string {
	return (*request).Header.Get("Origin")
}

var AllowedOrigins = &[]string{
	"https://localhost:3443",
	utils.EnvOrDefault("FRONTEND_ORIGIN_LOCAL", "https://localhost:8443"),
	"https://mars.local:3443",
	utils.EnvOrDefault("FRONTEND_ORIGIN", "https://mars.local:8443"),
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

const (
	ContentType     = "Content-Type"
	ApplicationJson = "application/json"
)

func WriteJson(writer *http.ResponseWriter, body interface{}) {
	(*writer).Header().Set(ContentType, ApplicationJson)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		(*writer).WriteHeader(http.StatusInternalServerError)
	} else if _, err = (*writer).Write(jsonBody); err != nil {
		(*writer).WriteHeader(http.StatusInternalServerError)
	}
}

func LogRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s \"%s %s\" \"%s\"\n", r.RemoteAddr, r.Method, r.URL, r.UserAgent())
		handler.ServeHTTP(w, r)
	})
}

func NewServer(mux *http.ServeMux, host, port string) *http.Server {
	return &http.Server{
		Addr:    fmt.Sprintf("%s:%s", host, port),
		Handler: LogRequest(mux),
	}
}
