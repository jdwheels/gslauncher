package status

type LaunchResponse struct {
	Status  string `json:"status"`
	Context string `json:"context"`
}

type ExtendedLaunchResponse struct {
	Status       string `json:"status"`
	Context      string `json:"context"`
	IsLaunched   bool   `json:"isLaunched"`
	IsTerminated bool   `json:"isTerminated"`
	Date         int64  `json:"date"`
}

func NewLaunchResponse(c, s string) *LaunchResponse {
	return &LaunchResponse{Context: c, Status: s}
}
