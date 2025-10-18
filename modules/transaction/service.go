package transaction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/utils"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
	"github.com/jmoiron/sqlx"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Service interface {
	// TODO: define service methods
	CreateTransaction(tx *sqlx.Tx, request CreateTransactionRequest) error
	GetListTransactionsPagination(request common.ParamsListRequest) (*response.Pagination[[]TransactionResponse], error)
	GetListTransactionsNoPagination(request common.ParamsListRequest) ([]TransactionResponse, error)
	GetOneTransaction(request *common.OneRequest) (*TransactionResponse, error)
	GetListTransactionsByUserId(request common.ParamsListRequest, userId int64, name string) (*response.Pagination[[]TransactionResponse], error)
	GetOneTransactionByUserId(request *common.OneRequest, userId int64, name string) (*TransactionResponse, error)
	UpdateOrderStatus(tx *sqlx.Tx, request UpdateOrderStatusRequest) error
	SetRatingMenu(tx *sqlx.Tx, request SetRatingMenuRequest) error
}

type transactionService struct {
	repo Repository
	db   *sqlx.DB
}

func NewTransactionService(repo Repository, db *sqlx.DB) Service {
	return &transactionService{repo: repo, db: db}
}

func (s *transactionService) CreateTransaction(tx *sqlx.Tx, request CreateTransactionRequest) error {
	// Convert menuIds slice to a comma-separated string
	var ids string
	for i, data := range request.Datas {
		if i > 0 {
			ids += ","
		}
		ids += fmt.Sprintf("%d", data.MenuID)
	}
	menus, err := getAvailableMenuByIdsAndTableById(ids, request.TableId)
	if err != nil {
		return err
	}

	if len(menus) != len(request.Datas) {
		return response.BadRequest("No menus found for the given IDs", nil)
	}

	request.Total = calculateTotalPriceMenu(menus, &request)

	err = paymentUseWallet(request.CreatedBy, request.Total, request.Pin)
	if err != nil {
		return err
	}

	id, err := s.repo.InsertThTransaction(tx, request)
	if err != nil {
		return err
	}

	for i := range request.Datas {
		err = s.repo.InsertTdTransaction(tx, id, request.CreatedBy, request.Datas[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *transactionService) GetListTransactionsPagination(request common.ParamsListRequest) (*response.Pagination[[]TransactionResponse], error) {
	res, err := s.repo.GetListTransactionsPagination(request)
	if err != nil {
		return nil, err
	}

	menuIds := []string{}
	tableIds := []string{}
	userIds := []string{}
	for _, data := range res.Data {
		tableIdStr := utils.Int64ToString(data.TableId)

		if data.TableId != 0 && !strings.Contains(strings.Join(tableIds, ","), tableIdStr) {
			tableIds = append(tableIds, tableIdStr)
		}

		if data.UserId != 0 && !strings.Contains(strings.Join(userIds, ","), utils.Int64ToString(data.UserId)) {
			userIds = append(userIds, utils.Int64ToString(data.UserId))
		}

		for _, detail := range data.Details {
			menuIdStr := utils.IntToString(detail.MenuId)
			if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
				menuIds = append(menuIds, menuIdStr)
			}
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")
	tableIdsStr := strings.Join(tableIds, ",")
	userIdsStr := strings.Join(userIds, ",")

	if menuIdsStr != "" && tableIdsStr != "" && userIdsStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAndTable(menuIdsStr, tableIdsStr)
		if err != nil {
			return nil, err
		}
		dataUsers, err := getUsersNameByIds(userIdsStr)
		if err != nil {
			return nil, err
		}

		for i, data := range res.Data {

			for idDataDetail, dataDetail := range data.Details {
				for _, menu := range dataMenusAndTable.Menus {
					if dataDetail.MenuId == menu.Id {
						res.Data[i].Details[idDataDetail].MenuName = menu.Name
						res.Data[i].Details[idDataDetail].Photo = menu.Photo
						res.Data[i].Details[idDataDetail].Description = menu.Description
						break
					}
				}
			}
			for _, table := range dataMenusAndTable.Tables {
				if data.TableId == table.Id {
					res.Data[i].TableName = table.Name
					break
				}
			}
			for _, user := range dataUsers {
				if data.UserId == user.UserId {
					res.Data[i].OrderBy = user.FullName
					break
				}
			}

		}

	}
	return res, nil
}

func (s *transactionService) GetListTransactionsNoPagination(request common.ParamsListRequest) ([]TransactionResponse, error) {
	res, err := s.repo.GetListTransactionsNoPagination(request)
	if err != nil {
		return nil, err
	}

	menuIds := []string{}
	tableIds := []string{}
	userIds := []string{}
	for _, data := range res {
		tableIdStr := utils.Int64ToString(data.TableId)

		if data.TableId != 0 && !strings.Contains(strings.Join(tableIds, ","), tableIdStr) {
			tableIds = append(tableIds, tableIdStr)
		}

		if data.UserId != 0 && !strings.Contains(strings.Join(userIds, ","), utils.Int64ToString(data.UserId)) {
			userIds = append(userIds, utils.Int64ToString(data.UserId))
		}

		for _, detail := range data.Details {
			menuIdStr := utils.IntToString(detail.MenuId)
			if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
				menuIds = append(menuIds, menuIdStr)
			}
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")
	tableIdsStr := strings.Join(tableIds, ",")
	userIdsStr := strings.Join(userIds, ",")

	if menuIdsStr != "" && tableIdsStr != "" && userIdsStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAndTable(menuIdsStr, tableIdsStr)
		if err != nil {
			return nil, err
		}
		dataUsers, err := getUsersNameByIds(userIdsStr)
		if err != nil {
			return nil, err
		}

		for i, data := range res {
			for idDataDetail, dataDetail := range data.Details {
				for _, menu := range dataMenusAndTable.Menus {
					if dataDetail.MenuId == menu.Id {
						res[i].Details[idDataDetail].MenuName = menu.Name
						res[i].Details[idDataDetail].Photo = menu.Photo
						res[i].Details[idDataDetail].Description = menu.Description
						break
					}
				}
			}
			for _, table := range dataMenusAndTable.Tables {
				if data.TableId == table.Id {
					res[i].TableName = table.Name
					break
				}
			}

			for _, user := range dataUsers {
				if data.UserId == user.UserId {
					res[i].OrderBy = user.FullName
					break
				}
			}

		}
	}

	return res, nil
}

func (s *transactionService) GetOneTransaction(request *common.OneRequest) (*TransactionResponse, error) {
	res, err := s.repo.GetOneTransaction(request.Id)
	if err != nil {
		return nil, err
	}

	menuIds := []string{}
	tableIdStr := utils.Int64ToString(res.TableId)
	userIdStr := utils.Int64ToString(res.UserId)

	for _, detail := range res.Details {
		menuIdStr := utils.IntToString(detail.MenuId)
		if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
			menuIds = append(menuIds, menuIdStr)
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")

	if menuIdsStr != "" && tableIdStr != "" && userIdStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAndTable(menuIdsStr, tableIdStr)
		if err != nil {
			return nil, err
		}
		dataUsers, err := getUsersNameByIds(userIdStr)
		if err != nil {
			return nil, err
		}

		for idDataDetail, dataDetail := range res.Details {
			for _, menu := range dataMenusAndTable.Menus {
				if dataDetail.MenuId == menu.Id {
					res.Details[idDataDetail].MenuName = menu.Name
					res.Details[idDataDetail].Photo = menu.Photo
					res.Details[idDataDetail].Description = menu.Description
					break
				}
			}
		}
		if res.TableId == dataMenusAndTable.Tables[0].Id {
			res.TableName = dataMenusAndTable.Tables[0].Name
		}
		if res.UserId == dataUsers[0].UserId {
			res.OrderBy = dataUsers[0].FullName
		}
	}

	return res, nil
}

func (s *transactionService) GetListTransactionsByUserId(request common.ParamsListRequest, userId int64, name string) (*response.Pagination[[]TransactionResponse], error) {
	res, err := s.repo.GetListTransactionsByUserId(request, userId)
	if err != nil {
		return nil, err
	}

	menuIds := []string{}
	tableIds := []string{}
	for _, data := range res.Data {
		tableIdStr := utils.Int64ToString(data.TableId)

		if data.TableId != 0 && !strings.Contains(strings.Join(tableIds, ","), tableIdStr) {
			tableIds = append(tableIds, tableIdStr)
		}

		for _, detail := range data.Details {
			menuIdStr := utils.IntToString(detail.MenuId)
			if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
				menuIds = append(menuIds, menuIdStr)
			}
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")
	tableIdsStr := strings.Join(tableIds, ",")
	if menuIdsStr != "" && tableIdsStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAndTable(menuIdsStr, tableIdsStr)
		if err != nil {
			return nil, err
		}

		for i, data := range res.Data {
			for idDataDetail, dataDetail := range data.Details {
				for _, menu := range dataMenusAndTable.Menus {
					if dataDetail.MenuId == menu.Id {
						res.Data[i].Details[idDataDetail].MenuName = menu.Name
						res.Data[i].Details[idDataDetail].Photo = menu.Photo
						res.Data[i].Details[idDataDetail].Description = menu.Description
						break
					}
				}
			}
			for _, table := range dataMenusAndTable.Tables {
				if data.TableId == table.Id {
					res.Data[i].TableName = table.Name
					break
				}
			}

			res.Data[i].OrderBy = name
		}
	}

	return res, nil
}

func (s *transactionService) GetOneTransactionByUserId(request *common.OneRequest, userId int64, name string) (*TransactionResponse, error) {
	res, err := s.repo.GetOneTransactionByUserId(request.Id, userId)
	if err != nil {
		return nil, err
	}

	menuIds := []string{}
	tableIdStr := utils.Int64ToString(res.TableId)

	for _, detail := range res.Details {
		menuIdStr := utils.IntToString(detail.MenuId)
		if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
			menuIds = append(menuIds, menuIdStr)
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")

	if menuIdsStr != "" && tableIdStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAndTable(menuIdsStr, tableIdStr)
		if err != nil {
			return nil, err
		}

		for idDataDetail, dataDetail := range res.Details {
			for _, menu := range dataMenusAndTable.Menus {
				if dataDetail.MenuId == menu.Id {
					res.Details[idDataDetail].MenuName = menu.Name
					res.Details[idDataDetail].Photo = menu.Photo
					res.Details[idDataDetail].Description = menu.Description
					break
				}
			}
		}
		if res.TableId == dataMenusAndTable.Tables[0].Id {
			res.TableName = dataMenusAndTable.Tables[0].Name
		}
	}

	res.OrderBy = name

	return res, nil
}

func (s *transactionService) UpdateOrderStatus(tx *sqlx.Tx, request UpdateOrderStatusRequest) error {
	err := s.repo.UpdateOrderStatus(tx, request.Id, request.UpdatedBy)
	if err != nil {
		return err
	}

	return nil
}

func (s *transactionService) SetRatingMenu(tx *sqlx.Tx, request SetRatingMenuRequest) error {
	idMenu, err := s.repo.SetRatingMenu(tx, request.Id, request.Rating, request.UpdatedBy)
	if err != nil {
		return err
	}

	ch, err := lib.GetChannel()
	if err != nil {
		log.Error("Failed to get channel:", err)
		return response.InternalServerError("Internal Server Error", nil)
	}
	payload := []byte(fmt.Sprintf(`{"id": %d, "rating": %d, "updatedBy": %d}`, idMenu, request.Rating, request.UpdatedBy))
	err = lib.SendMessage(ch, "menu.set_rating", "menu.set_rating", "", lib.ExchangeDirect, amqp.Publishing{
		ContentType: "application/json",
		Body:        payload,
	}, string(payload), true, false, false, amqp.Table{})
	if err != nil {
		return err
	}

	return nil
}

func calculateTotalPriceMenu(menus []MenuResponse, request *CreateTransactionRequest) float64 {
	var total float64
	for _, menu := range menus {
		for iD, data := range request.Datas {
			if menu.Id == data.MenuID {
				request.Datas[iD].Price = menu.Price
				request.Datas[iD].Total = menu.Price * float64(data.Qty)
				total += menu.Price * float64(data.Qty)
			}
		}
	}

	return total
}

func createSignature(params string, body string, timestamp string) (string, error) {

	message := params + timestamp + body
	signature, err := utils.GenerateHMAC(message)
	if err != nil {
		log.Error("Failed to generate HMAC:", err)
		return "", response.InternalServerError("Internal Server Error", nil)
	}

	return signature, nil
}

func paymentUseWallet(userId int64, total float64, pin string) error {
	// Implement the logic to check the user's wallet balance
	urlWallet := fmt.Sprintf("%s/api/internal/pay", config.Config.ServiceWalletUrl)

	// Create the request body
	bodyRequest := PaymentRequest{
		UserId: userId,
		Amount: total,
		Pin:    pin,
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Marshal ke JSON (sekali saja)
	bodyBytes, err := json.Marshal(bodyRequest)
	if err != nil {
		return err
	}

	// Simpan versi string-nya untuk signature
	bodyString := string(bodyBytes)

	signature, err := createSignature("", bodyString, timestamp)

	if err != nil {
		return err
	}

	_, err = utils.InternalRequest(signature, timestamp, urlWallet, "POST", bytes.NewReader(bodyBytes))

	if err != nil {
		return err
	}

	return nil
}

func getAvailableMenuByIdsAndTableById(ids string, tableId int64) ([]MenuResponse, error) {
	urlMasterData := fmt.Sprintf("%s/api/internal/available-menus-table?ids=%s&tableId=%d", config.Config.ServiceMasterDataUrl, ids, tableId)

	params := url.Values{}
	params.Add("ids", ids)
	params.Add("tableId", fmt.Sprintf("%d", tableId))

	timestamp := time.Now().UTC().Format(time.RFC3339)

	signature, err := createSignature(params.Encode(), "", timestamp)

	if err != nil {
		return nil, err
	}

	body, err := utils.InternalRequest(signature, timestamp, urlMasterData, "GET", nil)
	if err != nil {
		return nil, err
	}

	var menus InternalMenuResponse
	err = json.Unmarshal(body, &menus)
	if err != nil {
		log.Error("Failed to unmarshal response body:", err)
		return nil, response.InternalServerError("Internal Server Error", nil)
	}

	return menus.Data, nil
}

func getDataMenuByIdsAndTable(ids string, tableIds string) (GetMenusAndTableResponse, error) {
	urlMasterData := fmt.Sprintf("%s/api/internal/data-menus-table?ids=%s&tableIds=%s", config.Config.ServiceMasterDataUrl, ids, tableIds)

	params := url.Values{}
	params.Add("ids", ids)
	params.Add("tableIds", tableIds)

	timestamp := time.Now().UTC().Format(time.RFC3339)

	signature, err := createSignature(params.Encode(), "", timestamp)

	if err != nil {
		return GetMenusAndTableResponse{}, err
	}

	body, err := utils.InternalRequest(signature, timestamp, urlMasterData, "GET", nil)
	if err != nil {
		return GetMenusAndTableResponse{}, err
	}

	var data InternalGetMenusAndTableResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Error("Failed to unmarshal response body:", err)
		return GetMenusAndTableResponse{}, response.InternalServerError("Internal Server Error", nil)
	}

	return data.Data, nil
}

func getUsersNameByIds(ids string) ([]UserResponse, error) {
	urlAccount := fmt.Sprintf("%s/api/internal/name-users?ids=%s", config.Config.ServiceAccountUrl, ids)
	params := url.Values{}
	params.Add("ids", ids)

	timestamp := time.Now().UTC().Format(time.RFC3339)

	signature, err := createSignature(params.Encode(), "", timestamp)

	if err != nil {
		return nil, err
	}

	body, err := utils.InternalRequest(signature, timestamp, urlAccount, "GET", nil)
	if err != nil {
		return nil, err
	}

	var data InternalGetUserResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Error("Failed to unmarshal response body:", err)
		return nil, response.InternalServerError("Internal Server Error", nil)
	}

	return data.Data, nil
}
