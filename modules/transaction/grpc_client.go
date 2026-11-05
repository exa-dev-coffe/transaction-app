package transaction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"eka-dev.cloud/transaction-service/config"
	inventorypb "eka-dev.cloud/transaction-service/pb/inventory"
	masterdatapb "eka-dev.cloud/transaction-service/pb/masterdata"
	walletpb "eka-dev.cloud/transaction-service/pb/wallet"
	"eka-dev.cloud/transaction-service/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	mdClientConn   *grpc.ClientConn
	mdClient       masterdatapb.MasterDataServiceClient
	mdClientOnce   sync.Once
	mdClientLock   sync.RWMutex

	wltClientConn  *grpc.ClientConn
	wltClient      walletpb.WalletServiceClient
	wltClientOnce  sync.Once
	wltClientLock  sync.RWMutex

	invClientConn  *grpc.ClientConn
	invClient      inventorypb.InventoryServiceClient
	invClientOnce  sync.Once
	invClientLock  sync.RWMutex
)

// getMasterDataGrpcClient returns the singleton gRPC client for MasterData
func getMasterDataGrpcClient() (masterdatapb.MasterDataServiceClient, error) {
	if !config.Config.EnableGrpc {
		return nil, fmt.Errorf("gRPC disabled in config")
	}

	mdClientLock.RLock()
	if mdClient != nil {
		defer mdClientLock.RUnlock()
		return mdClient, nil
	}
	mdClientLock.RUnlock()

	mdClientLock.Lock()
	defer mdClientLock.Unlock()

	if mdClient != nil {
		return mdClient, nil
	}

	addr := config.Config.ServiceMasterDataGrpcUrl
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for master-data at %s: %w", addr, err)
	}

	mdClientConn = conn
	mdClient = masterdatapb.NewMasterDataServiceClient(conn)
	slog.Info("Initialized MasterData gRPC client", "address", addr)
	return mdClient, nil
}

// getWalletGrpcClient returns the singleton gRPC client for Wallet
func getWalletGrpcClient() (walletpb.WalletServiceClient, error) {
	if !config.Config.EnableGrpc {
		return nil, fmt.Errorf("gRPC disabled in config")
	}

	wltClientLock.RLock()
	if wltClient != nil {
		defer wltClientLock.RUnlock()
		return wltClient, nil
	}
	wltClientLock.RUnlock()

	wltClientLock.Lock()
	defer wltClientLock.Unlock()

	if wltClient != nil {
		return wltClient, nil
	}

	addr := config.Config.ServiceWalletGrpcUrl
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for wallet at %s: %w", addr, err)
	}

	wltClientConn = conn
	wltClient = walletpb.NewWalletServiceClient(conn)
	slog.Info("Initialized Wallet gRPC client", "address", addr)
	return wltClient, nil
}

// Resilient helper: Try gRPC first, fallback to HTTP REST for available menus
func getAvailableMenusResilient(ctx context.Context, ids string, tableId int64) ([]MenuResponse, error) {
	client, err := getMasterDataGrpcClient()
	if err == nil {
		grpcCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		resp, err := client.GetAvailableMenusAndValidateTable(grpcCtx, &masterdatapb.GetMenusAvailableAndValidateTableRequest{
			Ids:     ids,
			TableId: tableId,
		})
		if err == nil && resp != nil {
			result := make([]MenuResponse, 0, len(resp.Menus))
			for _, m := range resp.Menus {
				var disc *DiscountDetail
				if m.Discount != nil {
					disc = &DiscountDetail{
						PromotionID:   m.Discount.PromotionId,
						PromotionName: m.Discount.PromotionName,
						DiscountType:  m.Discount.DiscountType,
						DiscountValue: m.Discount.DiscountValue,
						Savings:       m.Discount.Savings,
					}
				}
				result = append(result, MenuResponse{
					Id:             int(m.Id),
					Price:          m.Price,
					EffectivePrice: m.EffectivePrice,
					Name:           m.Name,
					Description:    m.Description,
					Photo:          m.Photo,
					Discount:       disc,
				})
			}
			slog.Debug("Fetched available menus via gRPC successfully", "count", len(result))
			return result, nil
		}
		slog.Warn("MasterData gRPC GetAvailableMenus failed, falling back to HTTP REST", "error", err)
	}

	return getAvailableMenuByIdsAndTableById(ids, tableId)
}

// Resilient helper: Try gRPC first, fallback to HTTP REST for menu & table details
func getDataMenuByIdsAndTableResilient(ctx context.Context, ids string, tableIds string) (GetMenusAndTableResponse, error) {
	client, err := getMasterDataGrpcClient()
	if err == nil {
		grpcCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		resp, err := client.GetMenusAndTable(grpcCtx, &masterdatapb.GetMenusAndTableRequest{
			Ids:      ids,
			TableIds: tableIds,
		})
		if err == nil && resp != nil {
			menus := make([]MenuResponse, 0, len(resp.Menus))
			for _, m := range resp.Menus {
				var disc *DiscountDetail
				if m.Discount != nil {
					disc = &DiscountDetail{
						PromotionID:   m.Discount.PromotionId,
						PromotionName: m.Discount.PromotionName,
						DiscountType:  m.Discount.DiscountType,
						DiscountValue: m.Discount.DiscountValue,
						Savings:       m.Discount.Savings,
					}
				}
				menus = append(menus, MenuResponse{
					Id:             int(m.Id),
					Price:          m.Price,
					EffectivePrice: m.EffectivePrice,
					Name:           m.Name,
					Description:    m.Description,
					Photo:          m.Photo,
					Discount:       disc,
				})
			}

			tables := make([]TableResponse, 0, len(resp.Tables))
			for _, t := range resp.Tables {
				tables = append(tables, TableResponse{
					Id:   t.Id,
					Name: t.Name,
				})
			}

			slog.Debug("Fetched menu & table details via gRPC successfully")
			return GetMenusAndTableResponse{
				Menus:  menus,
				Tables: tables,
			}, nil
		}
		slog.Warn("MasterData gRPC GetMenusAndTable failed, falling back to HTTP REST", "error", err)
	}

	return getDataMenuByIdsAndTable(ids, tableIds)
}

// Resilient helper: Try gRPC first, fallback to HTTP REST for wallet payment
func paymentUseWalletResilient(ctx context.Context, userId int64, total float64, pin string) error {
	client, err := getWalletGrpcClient()
	if err == nil {
		grpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		resp, err := client.Pay(grpcCtx, &walletpb.PaymentRequest{
			UserId: userId,
			Amount: total,
			Pin:    pin,
		})
		if err == nil && resp != nil {
			if resp.Success {
				slog.Debug("Wallet payment succeeded via gRPC", "userId", userId, "total", total)
				return nil
			}
			return fmt.Errorf("wallet payment error: %s", resp.Message)
		}
		slog.Warn("Wallet gRPC Pay failed, falling back to HTTP REST", "error", err)
	}

	return paymentUseWallet(userId, total, pin)
}

// Resilient helper: Try gRPC first, fallback to HTTP REST for POS wallet payment
func payPosWalletResilient(ctx context.Context, paymentCode string, amount float64, orderId int64) (*PosWalletPayResponse, error) {
	client, err := getWalletGrpcClient()
	if err == nil {
		grpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		resp, err := client.PayWithPosCode(grpcCtx, &walletpb.PosWalletPayRequest{
			PaymentCode: paymentCode,
			Amount:      amount,
			OrderId:     orderId,
		})
		if err == nil && resp != nil {
			slog.Debug("POS wallet payment succeeded via gRPC", "orderId", orderId)
			return &PosWalletPayResponse{
				UserId:           resp.UserId,
				CustomerName:     resp.CustomerName,
				CustomerEmail:    resp.CustomerEmail,
				AmountPaid:       resp.AmountPaid,
				RemainingBalance: resp.RemainingBalance,
			}, nil
		}
		slog.Warn("Wallet gRPC PayWithPosCode failed, falling back to HTTP REST", "error", err)
	}

	return payPosWallet(paymentCode, amount, orderId)
}

// Resilient helper: Try gRPC first, fallback to HTTP REST for POS wallet refund
func refundPosWalletResilient(ctx context.Context, userId int64, amount float64, orderId int64) error {
	client, err := getWalletGrpcClient()
	if err == nil {
		grpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		resp, err := client.RefundPosWallet(grpcCtx, &walletpb.PosWalletRefundRequest{
			UserId:  userId,
			Amount:  amount,
			OrderId: orderId,
		})
		if err == nil && resp != nil && resp.Success {
			slog.Info("Successfully auto-refunded POS wallet payment via gRPC", "userId", userId, "orderId", orderId, "amount", amount)
			return nil
		}
		slog.Warn("Wallet gRPC RefundPosWallet failed, falling back to HTTP REST", "error", err)
	}

	return refundPosWallet(userId, amount, orderId)
}

// getInventoryGrpcClient returns the singleton gRPC client for Inventory
func getInventoryGrpcClient() (inventorypb.InventoryServiceClient, error) {
	if !config.Config.EnableGrpc {
		return nil, fmt.Errorf("gRPC disabled in config")
	}

	invClientLock.RLock()
	if invClient != nil {
		defer invClientLock.RUnlock()
		return invClient, nil
	}
	invClientLock.RUnlock()

	invClientLock.Lock()
	defer invClientLock.Unlock()

	if invClient != nil {
		return invClient, nil
	}

	addr := config.Config.ServiceInventoryGrpcUrl
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for inventory at %s: %w", addr, err)
	}

	invClientConn = conn
	invClient = inventorypb.NewInventoryServiceClient(conn)
	slog.Info("Initialized Inventory gRPC client", "address", addr)
	return invClient, nil
}

// Resilient helper: Try gRPC first, fallback to HTTP REST for stock availability check
func checkStockResilient(ctx context.Context, items []*inventorypb.OrderItemPayload) (*inventorypb.CheckStockResponse, error) {
	client, err := getInventoryGrpcClient()
	if err == nil {
		grpcCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		resp, err := client.CheckStockAvailability(grpcCtx, &inventorypb.CheckStockRequest{Items: items})
		if err == nil && resp != nil {
			slog.Debug("Checked stock availability via gRPC successfully", "isAvailable", resp.IsAvailable)
			return resp, nil
		}
		slog.Warn("Inventory gRPC CheckStockAvailability failed, falling back to HTTP REST", "error", err)
	}

	return checkStockHttp(items)
}

func checkStockHttp(items []*inventorypb.OrderItemPayload) (*inventorypb.CheckStockResponse, error) {
	type itemDto struct {
		MenuID   int64 `json:"menu_id"`
		Quantity int32 `json:"quantity"`
	}
	payloadItems := make([]itemDto, 0, len(items))
	for _, it := range items {
		payloadItems = append(payloadItems, itemDto{
			MenuID:   it.MenuId,
			Quantity: it.Quantity,
		})
	}

	reqBody := map[string]interface{}{
		"items": payloadItems,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal stock check request: %w", err)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	sig, err := createSignature("", string(bodyBytes), timestamp)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/internal/stock/check", config.Config.ServiceInventoryUrl)
	respBytes, err := utils.InternalRequest(sig, timestamp, url, "POST", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	type checkHttpResp struct {
		Success bool `json:"success"`
		Data    struct {
			IsAvailable  bool `json:"is_available"`
			MissingItems []struct {
				MenuID         int64   `json:"menu_id"`
				IngredientName string  `json:"ingredient_name"`
				StockAvailable float64 `json:"stock_available"`
				StockNeeded    float64 `json:"stock_needed"`
			} `json:"missing_items"`
		} `json:"data"`
	}

	var res checkHttpResp
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stock check response: %w", err)
	}

	missing := make([]*inventorypb.UnavailableItem, 0, len(res.Data.MissingItems))
	for _, m := range res.Data.MissingItems {
		missing = append(missing, &inventorypb.UnavailableItem{
			MenuId:         m.MenuID,
			IngredientName: m.IngredientName,
			StockAvailable: m.StockAvailable,
			StockNeeded:    m.StockNeeded,
		})
	}

	return &inventorypb.CheckStockResponse{
		IsAvailable:  res.Data.IsAvailable,
		MissingItems: missing,
	}, nil
}

// Resilient helper: Try gRPC first, fallback to HTTP REST for stock deduction
func deductStockResilient(ctx context.Context, transactionId string, items []*inventorypb.OrderItemPayload) error {
	client, err := getInventoryGrpcClient()
	if err == nil {
		grpcCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		resp, err := client.DeductStock(grpcCtx, &inventorypb.DeductStockRequest{
			TransactionId: transactionId,
			Items:         items,
		})
		if err == nil && resp != nil && resp.Success {
			slog.Debug("Deducted stock via gRPC successfully", "transactionId", transactionId)
			return nil
		}
		slog.Warn("Inventory gRPC DeductStock failed, falling back to HTTP REST", "error", err)
	}

	return deductStockHttp(transactionId, items)
}

func deductStockHttp(transactionId string, items []*inventorypb.OrderItemPayload) error {
	type itemDto struct {
		MenuID   int64 `json:"menu_id"`
		Quantity int32 `json:"quantity"`
	}
	payloadItems := make([]itemDto, 0, len(items))
	for _, it := range items {
		payloadItems = append(payloadItems, itemDto{
			MenuID:   it.MenuId,
			Quantity: it.Quantity,
		})
	}

	reqBody := map[string]interface{}{
		"transaction_id": transactionId,
		"items":          payloadItems,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal deduct stock request: %w", err)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	sig, err := createSignature("", string(bodyBytes), timestamp)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/internal/stock/deduct", config.Config.ServiceInventoryUrl)
	respBytes, err := utils.InternalRequest(sig, timestamp, url, "POST", bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	type deductHttpResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	var res deductHttpResp
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return fmt.Errorf("failed to unmarshal deduct stock response: %w", err)
	}
	if !res.Success {
		return fmt.Errorf("deduct stock failed: %s", res.Message)
	}

	return nil
}

// Resilient helper: Try gRPC first, fallback to HTTP REST for stock rollback
func rollbackStockResilient(ctx context.Context, transactionId string, items []*inventorypb.OrderItemPayload) error {
	client, err := getInventoryGrpcClient()
	if err == nil {
		grpcCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		resp, err := client.RollbackStock(grpcCtx, &inventorypb.RollbackStockRequest{
			TransactionId: transactionId,
			Items:         items,
		})
		if err == nil && resp != nil && resp.Success {
			slog.Debug("Rolled back stock via gRPC successfully", "transactionId", transactionId)
			return nil
		}
		slog.Warn("Inventory gRPC RollbackStock failed, falling back to HTTP REST", "error", err)
	}

	return rollbackStockHttp(transactionId, items)
}

func rollbackStockHttp(transactionId string, items []*inventorypb.OrderItemPayload) error {
	type itemDto struct {
		MenuID   int64 `json:"menu_id"`
		Quantity int32 `json:"quantity"`
	}
	payloadItems := make([]itemDto, 0, len(items))
	for _, it := range items {
		payloadItems = append(payloadItems, itemDto{
			MenuID:   it.MenuId,
			Quantity: it.Quantity,
		})
	}

	reqBody := map[string]interface{}{
		"transaction_id": transactionId,
		"items":          payloadItems,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal rollback stock request: %w", err)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	sig, err := createSignature("", string(bodyBytes), timestamp)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/internal/stock/rollback", config.Config.ServiceInventoryUrl)
	respBytes, err := utils.InternalRequest(sig, timestamp, url, "POST", bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	type rollbackHttpResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	var res rollbackHttpResp
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return fmt.Errorf("failed to unmarshal rollback stock response: %w", err)
	}
	if !res.Success {
		return fmt.Errorf("rollback stock failed: %s", res.Message)
	}

	return nil
}
