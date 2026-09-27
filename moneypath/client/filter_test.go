package client

import (
	"testing"

	"github.com/pranavagiligar/ezbookkeeping_tools/moneypath/models"
)

func TestApplyTransactionFilters(t *testing.T) {
	items := []models.EzTransaction{
		{
			ID:      "1",
			Type:    models.TransactionTypeExpense,
			Comment: "Lunch at Cafe Corner",
			TagIDs:  []string{"food", "office"},
			Category: &models.TransactionCategoryInfo{
				Name: "Dining",
			},
		},
		{
			ID:      "2",
			Type:    models.TransactionTypeIncome,
			Comment: "Salary deposit",
			TagIDs:  []string{"salary"},
			Category: &models.TransactionCategoryInfo{
				Name: "Salary",
			},
		},
		{
			ID:      "3",
			Type:    models.TransactionTypeTransfer,
			Comment: "Moved money to savings",
			TagIDs:  []string{"home"},
			Category: &models.TransactionCategoryInfo{
				Name: "Housing",
			},
		},
	}

	filtered := applyTransactionFilters(items, models.FilterParams{
		Types:               []int{models.TransactionTypeExpense, models.TransactionTypeTransfer},
		CategoryNames:       []string{"Dining"},
		TagNames:            []string{"food"},
		DescriptionContains: "cafe",
	})

	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(filtered))
	}
	if filtered[0].ID != "1" {
		t.Fatalf("expected first transaction to remain, got %q", filtered[0].ID)
	}
}
