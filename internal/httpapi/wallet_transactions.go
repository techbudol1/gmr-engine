package httpapi

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type walletTransaction struct {
	Asset           string `json:"asset"`
	BlockNumber     string `json:"blockNumber"`
	Category        string `json:"category"`
	Direction       string `json:"direction"`
	From            string `json:"from"`
	Timestamp       string `json:"timestamp"`
	To              string `json:"to"`
	TokenAddress    string `json:"tokenAddress,omitempty"`
	TransactionHash string `json:"transactionHash"`
	UniqueID        string `json:"uniqueId"`
	Value           string `json:"value"`
}

type alchemyAssetTransfer struct {
	Asset       string `json:"asset"`
	BlockNum    string `json:"blockNum"`
	Category    string `json:"category"`
	From        string `json:"from"`
	Hash        string `json:"hash"`
	Metadata    struct {
		BlockTimestamp string `json:"blockTimestamp"`
	} `json:"metadata"`
	RawContract struct {
		Address string `json:"address"`
	} `json:"rawContract"`
	To       string `json:"to"`
	UniqueID string `json:"uniqueId"`
	Value    any    `json:"value"`
}

func (s Server) walletTransactions(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	chainID, walletAddress, err := balanceQuery(c)
	if err != nil {
		return err
	}
	if err := enforceProjectPolicy(principal.App, chainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	limit := 25
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsed <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, "valid limit is required")
		}
		limit = parsed
	}
	if limit > 100 {
		limit = 100
	}

	inbound, inboundErr := s.alchemyWalletTransactions(c.Context(), chainID, walletAddress, "received", limit)
	outbound, outboundErr := s.alchemyWalletTransactions(c.Context(), chainID, walletAddress, "sent", limit)
	if inboundErr != nil && outboundErr != nil {
		return fiber.NewError(fiber.StatusBadGateway, "wallet transaction history is unavailable: "+inboundErr.Error())
	}
	transactions := append(inbound, outbound...)
	sort.SliceStable(transactions, func(i, j int) bool {
		blockComparison := compareHexQuantity(transactions[i].BlockNumber, transactions[j].BlockNumber)
		if blockComparison != 0 {
			return blockComparison > 0
		}
		return transactions[i].UniqueID > transactions[j].UniqueID
	})
	if len(transactions) > limit {
		transactions = transactions[:limit]
	}
	return c.JSON(fiber.Map{
		"chainId":       chainID,
		"transactions":  transactions,
		"walletAddress": strings.ToLower(walletAddress),
	})
}

func (s Server) alchemyWalletTransactions(ctx context.Context, chainID int64, walletAddress string, direction string, limit int) ([]walletTransaction, error) {
	params := map[string]any{
		"category":     []string{"external", "erc20"},
		"excludeZeroValue": true,
		"fromBlock":    "0x0",
		"maxCount":     "0x" + strconv.FormatInt(int64(limit), 16),
		"order":        "desc",
		"toBlock":      "latest",
		"withMetadata": true,
	}
	if direction == "received" {
		params["toAddress"] = walletAddress
	} else {
		params["fromAddress"] = walletAddress
	}
	var response struct {
		Transfers []alchemyAssetTransfer `json:"transfers"`
	}
	if err := s.rpc(ctx, chainID, "alchemy_getAssetTransfers", []any{params}, &response); err != nil {
		return nil, err
	}
	transactions := make([]walletTransaction, 0, len(response.Transfers))
	for _, transfer := range response.Transfers {
		value := ""
		if transfer.Value != nil {
			value = fmt.Sprint(transfer.Value)
		}
		uniqueID := strings.TrimSpace(transfer.UniqueID)
		if uniqueID == "" {
			uniqueID = transfer.Hash + ":" + transfer.Category + ":" + direction
		}
		transactions = append(transactions, walletTransaction{
			Asset:           transfer.Asset,
			BlockNumber:     transfer.BlockNum,
			Category:        transfer.Category,
			Direction:       direction,
			From:            strings.ToLower(transfer.From),
			Timestamp:       transfer.Metadata.BlockTimestamp,
			To:              strings.ToLower(transfer.To),
			TokenAddress:    strings.ToLower(transfer.RawContract.Address),
			TransactionHash: transfer.Hash,
			UniqueID:        uniqueID,
			Value:           value,
		})
	}
	return transactions, nil
}
