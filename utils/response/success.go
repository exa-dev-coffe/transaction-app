package response

import "time"

type Response struct {
	Message   string      `json:"message"`
	Data      interface{} `json:"data"`
	Success   bool        `json:"success"`
	TimeStamp time.Time   `json:"timestamp"`
}

type Pagination struct {
	Data        interface{} `json:"data"`
	TotalData   int         `json:"totalData"`
	TotalPages  int         `json:"totalPages"`
	CurrentPage int         `json:"currentPage"`
	PageSize    int         `json:"pageSize"`
	LastPage    bool        `json:"lastPage"`
}

func Success(message string, data interface{}) Response {
	return Response{
		Message:   message,
		Data:      data,
		Success:   true,
		TimeStamp: time.Now(),
	}
}
