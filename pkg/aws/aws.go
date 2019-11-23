package aws

import (
	_status "defilade.io/gslauncher/pkg/status"
	"defilade.io/gslauncher/pkg/utils"
	"defilade.io/gslauncher/pkg/web"
	"log"
	"net/http"
)

var isProd = utils.EnvOrDefault("GOENV", "dev") == "production"

func actionWrapper(target string, action func(string) bool) bool {
	log.Printf("GOENV => %s => isProd => %t", utils.EnvOrDefault("GOENV", "dev"), isProd)
	if isProd {
		return action(target)
	}
	return dryAction(target)
}

func dryAction(target string) bool {
	log.Printf("Simulating action on '%s'", target)
	return true
}

func Event(writer *http.ResponseWriter, status string, toggle func()) {
	toggle()
	web.WriteJson(writer, _status.NewLaunchResponse(status))
}

func Action(writer *http.ResponseWriter, target string, action func(string) bool, status string, toggle func()) {
	if success := actionWrapper(target, action); success {
		toggle()
		web.WriteJson(writer, _status.NewLaunchResponse(status))
	} else {
		(*writer).WriteHeader(http.StatusInternalServerError)
	}
}
