package sideStrategy

import (
	"fmt"

	"github.com/lightyeario/kelp/api"
	"github.com/lightyeario/kelp/model"
	"github.com/lightyeario/kelp/plugins"
	"github.com/stellar/go/build"
	"github.com/stellar/go/clients/horizon"
	"github.com/stellar/go/support/log"
)

// DeleteSideStrategy is a sideStrategy to delete the orders for a given currency pair on one side of the orderbook
type DeleteSideStrategy struct {
	sdex       *plugins.SDEX
	assetBase  *horizon.Asset
	assetQuote *horizon.Asset
}

// ensure it implements SideStrategy
var _ api.SideStrategy = &DeleteSideStrategy{}

// MakeDeleteSideStrategy is a factory method for DeleteSideStrategy
func MakeDeleteSideStrategy(
	sdex *plugins.SDEX,
	assetBase *horizon.Asset,
	assetQuote *horizon.Asset,
) api.SideStrategy {
	return &DeleteSideStrategy{
		sdex:       sdex,
		assetBase:  assetBase,
		assetQuote: assetQuote,
	}
}

// PruneExistingOffers impl
func (s *DeleteSideStrategy) PruneExistingOffers(offers []horizon.Offer) ([]build.TransactionMutator, []horizon.Offer) {
	log.Info(fmt.Sprintf("deleteSideStrategy: deleting %d offers", len(offers)))
	pruneOps := []build.TransactionMutator{}
	for i := 0; i < len(offers); i++ {
		pOp := s.sdex.DeleteOffer(offers[i])
		pruneOps = append(pruneOps, &pOp)
	}
	return pruneOps, []horizon.Offer{}
}

// PreUpdate impl
func (s *DeleteSideStrategy) PreUpdate(maxAssetBase float64, maxAssetQuote float64, trustBase float64, trustQuote float64) error {
	return nil
}

// UpdateWithOps impl
func (s *DeleteSideStrategy) UpdateWithOps(offers []horizon.Offer) (ops []build.TransactionMutator, newTopOffer *model.Number, e error) {
	return []build.TransactionMutator{}, nil, nil
}

// PostUpdate impl
func (s *DeleteSideStrategy) PostUpdate() error {
	return nil
}
