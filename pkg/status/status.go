package status

type LaunchResponse struct {
	Status string `json:"status"`
}

type ExtendedLaunchResponse struct {
	Status       string `json:"status"`
	IsLaunched   bool   `json:"isLaunched"`
	IsTerminated bool   `json:"isTerminated"`
	Date         int64  `json:"date"`
}

func NewLaunchResponse(s string) *LaunchResponse {
	return &LaunchResponse{Status: s}
}