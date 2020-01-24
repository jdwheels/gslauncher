package main

import (
	"context"
	_aws "defilade.io/gslauncher/pkg/aws"
	"defilade.io/gslauncher/pkg/sse"
	_status "defilade.io/gslauncher/pkg/status"
	"defilade.io/gslauncher/pkg/utils"
	"defilade.io/gslauncher/pkg/web"
	"github.com/gorilla/mux"
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

//var clusterName = utils.EnvOrDefault("CLUSTER_NAME", "EC2ContainerService-game-servers-2-EcsInstanceAsg-9AB2NHDSISGL")

type ServerConfig struct {
	AutoScalingGroup string
	Domain           string
}

var clusterNames = map[string]*ServerConfig{
	"arma":   {"EC2ContainerService-game-servers-2-EcsInstanceAsg-9AB2NHDSISGL", "arma.defilade.io"},
	"mumble": {"asg-docker-mumble", "mumble2.defilade.io"},
}

func getServerConfig(writer http.ResponseWriter, req *http.Request) (config *ServerConfig, err error) {
	vars := mux.Vars(req)
	config, ok := clusterNames[vars["name"]]
	if !ok {
		writer.WriteHeader(http.StatusBadRequest)
	}
	return
}

func initial(writer http.ResponseWriter, req *http.Request) {
	config, err := getServerConfig(writer, req)
	if err != nil {
		return
	}
	count, status, err := xaws.CheckIt(config.AutoScalingGroup)
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

func launch(writer http.ResponseWriter, req *http.Request) {
	config, err := getServerConfig(writer, req)
	if err != nil {
		return
	}
	_aws.Action(&writer, config.AutoScalingGroup, xaws.StartEC2Cluster, "Pending", func() {
		isLaunched = false
		isTerminated = false
	})
}

func handleAwsBody(req *http.Request) (name string, err error) {
	body := &_aws.LambdaBody{}
	err = web.ReadJson(req, body)
	if err != nil {
		log.Printf("Error parsing body %v", err)
		return
	}
	name, found := findContextName(body)
	if !found {
		log.Printf("Error finding context from asg %s", body.Asg)
	}
	return
}

func findContextName(a *_aws.LambdaBody) (name string, found bool) {
	found = false
	for k, v := range clusterNames {
		if v.AutoScalingGroup == a.Asg {
			name = k
			found = true
			break
		}
	}
	return
}

func launched(broker *sse.Broker) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
		n, err := handleAwsBody(req)
		if err != nil {
			return
		}
		broker.SimpleEvent(n, "launched")
		_aws.Event(&writer, n, "Ok", func() {
			isTerminated = false
			isLaunched = true
		})
	}
}

func terminate(writer http.ResponseWriter, req *http.Request) {
	config, err := getServerConfig(writer, req)
	if err != nil {
		return
	}
	_aws.Action(&writer, config.AutoScalingGroup, xaws.StopEC2Cluster, "Terminating", func() {
		isTerminated = false
		isLaunched = false
	})
}

func terminated(broker *sse.Broker) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
		n, err := handleAwsBody(req)
		if err != nil {
			return
		}
		broker.SimpleEvent(n, "terminated")
		_aws.Event(&writer, n, "Ok", func() {
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

	certDir := utils.EnvOrDefault("CERT_DIR", "/home/john/Projects/cert-scripts")
	certName := utils.EnvOrDefault("CERT_NAME", "ss3")
	cert := path.Join(certDir, certName)
	f = func() error {
		return server.ListenAndServeTLS(cert+".crt", cert+".key")
	}
	return
}

func configureServer(handler http.Handler, host, port string) (server *http.Server, f func() error) {
	server = web.NewServer(handler, host, port)

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

func clusters(writer http.ResponseWriter, _ *http.Request) {
	var names []string
	for name := range clusterNames {
		names = append(names, name)
	}
	web.WriteJson(&writer, names)
}

func main() {
	idleConnsClosed := make(chan struct{})
	muxS := mux.NewRouter()
	s := muxS.PathPrefix("/servers/{name}").Subrouter()
	s.HandleFunc("/status", web.Get(initial))
	s.HandleFunc("/status-x", web.Get(initial))
	s.HandleFunc("/launch", web.Post(launch))
	s.HandleFunc("/terminate", web.Post(terminate))
	muxS.HandleFunc("/clusters", web.Get(clusters))

	muxS.HandleFunc("/err", web.Get(errTest))
	host := utils.EnvOrDefault("HOST", "0.0.0.0")
	port := utils.EnvOrDefault("PORT", "9443")
	server, fS := configureServer(muxS, host, port)

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
