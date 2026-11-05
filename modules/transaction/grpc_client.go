package transaction

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"eka-dev.cloud/transaction-service/config"
	masterdatapb "eka-dev.cloud/transaction-service/pb/masterdata"
	walletpb "eka-dev.cloud/transaction-service/pb/wallet"
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
