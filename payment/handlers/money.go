package handlers

import (
	"math"

	pdomain "encore.app/payment/domain"
)

func transactionAmountCents(txn *pdomain.Transaction) int64 {
	if txn == nil {
		return 0
	}
	if txn.AmountCents > 0 {
		return txn.AmountCents
	}
	return amountToCents(txn.Amount)
}

func amountToCents(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

func amountCentsToMajorUnits(amountCents int64) float64 {
	return float64(amountCents) / 100.0
}
