package main

import (
	"context"
	_aws "defilade.io/gslauncher/pkg/aws"
	"defilade.io/gslauncher/pkg/sse"
	_status "defilade.io/gslauncher/pkg/status"
	"defilade.io/gslauncher/pkg/utils"
	"defilade.io/gslauncher/pkg/web"
	xaws "github.com/jdwheels/xaws/pkg/ec2"
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

var isTerminated = true
var isLaunched = false
var clusterName = utils.EnvOrDefault("CLUSTER_NAME", "EC2ContainerService-game-servers-2-EcsInstanceAsg-9AB2NHDSISGL")

var clusterNames = map[string]string {
	"arma": "EC2ContainerService-game-servers-2-EcsInstanceAsg-9AB2NHDSISGL",
	"mumble": "asg-docker-mumble",
}

func initial(writer http.ResponseWriter, _ *http.Request) {
	count, status, err := xaws.CheckIt(clusterName)
	if err == nil {
		isLaunched = count > 0 && status == "InService"
		isTerminated = count == 0 || status == "Terminating:Proceed"
	}
	web.WriteJson(&writer, _status.ExtendedLaunchResponse{
		Status:       "N/A",
		IsLaunched:   isLaunched,
		IsTerminated: isTerminated,
		Date:         time.Now().Unix(),
	})
}

func launch(writer http.ResponseWriter, _ *http.Request) {
	_aws.Action(&writer, clusterName, xaws.StartEC2Cluster, "Pending", func() {
		isLaunched = false
		isTerminated = false
	})
}

func launched(broker *sse.Broker) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		broker.SimpleEvent("launched")
		_aws.Event(&writer, "Ok", func() {
			isTerminated = false
			isLaunched = true
		})
	}
}

func terminate(writer http.ResponseWriter, _ *http.Request) {
	_aws.Action(&writer, clusterName, xaws.StopEC2Cluster, "Terminating", func() {
		isTerminated = false
		isLaunched = false
	})
}

func terminated(broker *sse.Broker) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		broker.SimpleEvent("terminated")
		_aws.Event(&writer, "Ok", func() {
			isLaunched = false
			isTerminated = true
		})
	}
}

func errTest(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusBadRequest)
}

func configureHttp2Server(server *http.Server) (f func() error, err error) {
	conf2 := http2.Server{}

	if err = http2.ConfigureServer(server, &conf2); err != nil {
		log.Fatalf("HTTP2 error %s", err)
	}

	certDir := utils.EnvOrDefault("CERT_DIR", "/home/john/algo/wpr/certs")
	certName := utils.EnvOrDefault("CERT_NAME", "selfsigned")
	cert := path.Join(certDir, certName)
	f = func() error {
		return server.ListenAndServeTLS(cert+".crt", cert+".key")
	}
	return
}

func configureServer(mux *http.ServeMux, host, port string) (server *http.Server, f func() error) {
	server = web.NewServer(mux, host, port)

	useHttp2, _ := strconv.ParseBool(utils.EnvOrDefault("USE_HTTP2", "true"))

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

func handleClose(idleConnsClosed chan struct{}, servers []*http.Server) {
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

func clusters(writer http.ResponseWriter, _ *http.Request)  {
	var names []string
	for name := range clusterNames {
		names = append(names, name)
	}
	web.WriteJson(&writer, names)
}

func main() {
	idleConnsClosed := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/status", web.Get(initial))
	mux.HandleFunc("/status-x", web.Get(initial))
	mux.HandleFunc("/launch", web.Post(launch))
	mux.HandleFunc("/terminate", web.Post(terminate))
	mux.HandleFunc("/clusters", web.Get(clusters))

	mux.HandleFunc("/err", web.Get(errTest))
	host := utils.EnvOrDefault("HOST", "0.0.0.0")
	port := utils.EnvOrDefault("PORT", "9443")
	server, fS := configureServer(mux, host, port)

	broker := sse.NewBroker()

	muxE := http.NewServeMux()
	muxE.Handle("/listen", broker)
	muxE.HandleFunc("/event", broker.Event)
	muxE.HandleFunc("/terminated", web.Post(terminated(broker)))
	muxE.HandleFunc("/launched", web.Post(launched(broker)))
	portE := utils.EnvOrDefault("PORT_E", "9444")
	serverE, fE := configureServer(muxE, host, portE)

	servers := []*http.Server{server, serverE}

	go handleClose(idleConnsClosed, servers)

	fs := []*func() error{&fS, &fE}

	for i := 0; i < len(fs); i++ {
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
