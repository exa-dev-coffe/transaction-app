package transaction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/utils"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
	"github.com/jmoiron/sqlx"
)

type Service interface {
	// TODO: define service methods
	CreateTransaction(tx *sqlx.Tx, request CreateTransactionRequest) error
	GetListTransactionsPagination(request common.ParamsListRequest) (*response.Pagination[[]TransactionResponse], error)
	GetListTransactionsNoPagination(request common.ParamsListRequest) ([]TransactionResponse, error)
	GetOneTransaction(request *common.OneRequest) (*TransactionResponse, error)
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
	for _, data := range res.Data {
		for _, detail := range data.Details {
			menuIdStr := utils.IntToString(detail.MenuId)
			if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
				menuIds = append(menuIds, menuIdStr)
			}
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")

	tableIds := []string{}
	for _, data := range res.Data {
		tableIdStr := utils.Int64ToString(data.TableId)
		if data.TableId != 0 && !strings.Contains(strings.Join(tableIds, ","), tableIdStr) {
			tableIds = append(tableIds, tableIdStr)
		}
	}

	tableIdsStr := strings.Join(tableIds, ",")

	if menuIdsStr != "" && tableIdsStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAbdTable(menuIdsStr, tableIdsStr)
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
				for _, table := range dataMenusAndTable.Tables {
					if data.TableId == table.Id {
						res.Data[i].TableName = table.Name
						break
					}
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
	for _, data := range res {
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
		dataMenusAndTable, err := getDataMenuByIdsAbdTable(menuIdsStr, tableIdsStr)
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
				for _, table := range dataMenusAndTable.Tables {
					if data.TableId == table.Id {
						res[i].TableName = table.Name
						break
					}
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

	for _, detail := range res.Details {
		menuIdStr := utils.IntToString(detail.MenuId)
		if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
			menuIds = append(menuIds, menuIdStr)
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")

	if menuIdsStr != "" && tableIdStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAbdTable(menuIdsStr, tableIdStr)
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

	return res, nil
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

func getDataMenuByIdsAbdTable(ids string, tableIds string) (GetMenusAndTableResponse, error) {
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

//func paymentUserBalance(total float64, userId int64) error {
//	url := fmt.Sprintf("%s/api/internal/users/%d/decrease_balance", config.Config.ServiceUserUrl, userId)
//
//	bodyRequest := map[string]float64{"amount": total}
//	bodyJson, err := json.Marshal(bodyRequest)
//	if err != nil {
//		log.Error("Failed to marshal body:", err)
//		return response.InternalServerError("Internal Server Error", nil)
//	}
//
//	timestamp := fmt.Sprintf("%d", time.Now().Unix())
//
//	message := timestamp + string(bodyJson)
//
//	signature, err := utils.GenerateHMAC(message)
//	if err != nil {
//		log.Error("Failed to generate HMAC:", err)
//		return response.InternalServerError("Internal Server Error", nil)
//	}
//
//	}

//
//unc sendInternalRequest() error {
//secret := "MY_SUPER_SECRET"
//url := "http://service-b.internal/api/process"
//
//body := []byte(`{"action":"update_balance"}`)
//timestamp := fmt.Sprintf("%d", time.Now().Unix())
//
//message := timestamp + string(body)
//signature := auth.GenerateHMAC(secret, message)
//
//req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
//if err != nil {
//return err
//}
//
//req.Header.Set("Content-Type", "application/json")
//req.Header.Set("X-Timestamp", timestamp)
//req.Header.Set("X-Signature", signature)
//
//client := &http.Client{Timeout: 5 * time.Second}
//res, err := client.Do(req)
//if err != nil {
//return err
//}
//defer res.Body.Close()
//
//fmt.Println("Status:", res.Status)
//return nil
//}
