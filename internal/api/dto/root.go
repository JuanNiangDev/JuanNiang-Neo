package dto

func GenFinalResponse(response Response, data interface{}) FinalResponse {
	return FinalResponse{Status: response.Status, Info: response.Info, Data: data}
}

type Response struct {
	Status uint   `json:"status"`
	Info   string `json:"info"`
}

func (r Response) Error() string {
	return r.Info
}

type FinalResponse struct {
	Status uint        `json:"status"`
	Info   string      `json:"info"`
	Data   interface{} `json:"data"`
}
