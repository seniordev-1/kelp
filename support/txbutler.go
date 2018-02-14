package kelp

import (
	"strconv"

	"github.com/pkg/errors"
	"github.com/stellar/go/build"
	"github.com/stellar/go/clients/horizon"
	"github.com/stellar/go/support/log"
)

const baseReserve = 0.5
const operationalBuffer = 2000

type TxButler struct {
	API            horizon.Client
	SourceAccount  string
	TradingAccount string
	SourceSeed     string
	TradingSeed    string
	Network        build.Network
	seqNum         uint64
	reloadSeqNum   bool

	// uninitialized
	cachedXlmExposure *float64
}

func (self *TxButler) Init() {
	//log.Info("init txbutler")
	log.Info("Using network passphrase: ", self.Network.Passphrase)

	if self.SourceAccount == "" {
		self.SourceAccount = self.TradingAccount
		self.SourceSeed = self.TradingSeed
		log.Info("No Source Account Set")
	}
	self.reloadSeqNum = true
}

/*
func (self *TxButler) SetSeqNum(num string) {
	s, err := strconv.ParseUint(num, 10, 64)
	if err != nil {
		log.Info("SetSeqNum :", num, " failed: ", err)
		return
	}
	self.seqNum = s
	self.reloadSeqNum = false
}
*/

func (self *TxButler) incrementSeqNum() {
	if self.reloadSeqNum {
		log.Info("reloadSeqNum ")
		seqNum, err := self.API.SequenceForAccount(self.SourceAccount)
		if err != nil {
			log.Info("error getting seq num ", err)
			return
		}
		self.seqNum = uint64(seqNum)
		self.reloadSeqNum = false
	}
	self.seqNum++

}

func (self *TxButler) DeleteAllOffers() {
	offers, err := LoadAllOffers(self.TradingAccount, self.API)
	if err != nil {
		log.Info("DeleteAllOffers: ", err)
		return
	}
	for _, offer := range offers {
		self.DeleteOffer(offer)
	}
}

// TODO 2 - make this return operations so this can be done in a single ledger without having to deal with seq number management across threads
func (self *TxButler) DeleteOffer(offer horizon.Offer) {
	//log.Info("Delete Offer: ", offer.ID)
	rate := build.Rate{
		Selling: Asset2Asset(offer.Selling),
		Buying:  Asset2Asset(offer.Buying),
		Price:   build.Price(offer.Price),
	}

	var mo build.ManageOfferBuilder
	if self.SourceAccount == self.TradingAccount {
		mo = build.ManageOffer(false, build.Amount("0"), rate, build.OfferID(offer.ID))
	} else {
		mo = build.ManageOffer(false, build.Amount("0"), rate, build.OfferID(offer.ID), build.SourceAccount{self.TradingAccount})
	}

	self.incrementSeqNum()
	tx := build.Transaction(
		build.SourceAccount{self.SourceAccount},
		build.Sequence{self.seqNum},
		self.Network,
		mo,
	)

	go self.signAndSubmit(tx)
}

func (self *TxButler) ModifyBuyOffer(offer horizon.Offer, price float64, amount float64) *build.ManageOfferBuilder {
	//log.Info("modifyBuyOffer: ", offer.ID, " p:", price)
	return self.ModifySellOffer(offer, 1/price, amount*price)
}

func (self *TxButler) ModifySellOffer(offer horizon.Offer, price float64, amount float64) *build.ManageOfferBuilder {
	//log.Info("modifySellOffer: ", offer.ID, " p:", amount)
	return self.createModifySellOffer(&offer, offer.Selling, offer.Buying, price, amount)
}

func (self *TxButler) CreateSellOffer(base horizon.Asset, counter horizon.Asset, price float64, amount float64) *build.ManageOfferBuilder {
	if amount > 0 {
		//log.Info("createSellOffer: ", price, amount)
		return self.createModifySellOffer(nil, base, counter, price, amount)
	}
	log.Info("zero amount ")
	return nil
}

// ParseOfferAmount is a convenience method to parse an offer amount created by the txButler
func (self *TxButler) ParseOfferAmount(amt string) (float64, error) {
	offerAmt, err := strconv.ParseFloat(amt, 64)
	if err != nil {
		log.Info("error parsing offer amount: ", err)
		return -1, err
	}
	return offerAmt, nil
}

func (self *TxButler) minReserve(subentries int32) float64 {
	return float64(float64(2+subentries) * baseReserve)
}

func (self *TxButler) lumenBalance() (float64, float64, error) {
	account, err := self.API.LoadAccount(self.TradingAccount)
	if err != nil {
		log.Info("error loading account to fetch lumen balance: ", err)
		return -1, -1, nil
	}

	for _, balance := range account.Balances {
		if balance.Asset.Type == "native" {
			b, e := strconv.ParseFloat(balance.Balance, 64)
			if e != nil {
				log.Info("error parsing native balance: ", e)
			}
			return b, self.minReserve(account.SubentryCount), e
		}
	}
	return -1, -1, errors.New("could not find a native lumen balance!")
}

func (self *TxButler) createModifySellOffer(offer *horizon.Offer, selling horizon.Asset, buying horizon.Asset, price float64, amount float64) *build.ManageOfferBuilder {
	if selling.Type == "native" {
		var incrementalXlmAmount float64
		if offer != nil {
			offerAmt, err := self.ParseOfferAmount(offer.Amount)
			if err != nil {
				return nil
			}
			// modifying an offer will not increase the min reserve but will affect the xlm exposure
			incrementalXlmAmount = amount - offerAmt
		} else {
			// creating a new offer will incrase the min reserve on the account so add baseReserve
			incrementalXlmAmount = amount + baseReserve
		}

		// check if incrementalXlmAmount is within budget
		bal, minAccountBal, err := self.lumenBalance()
		if err != nil {
			return nil
		}

		xlmExposure, err := self.xlmExposure()
		if err != nil {
			return nil
		}

		additionalExposure := incrementalXlmAmount >= 0
		possibleTerminalExposure := (xlmExposure + incrementalXlmAmount) > (bal - minAccountBal - operationalBuffer)
		if additionalExposure && possibleTerminalExposure {
			log.Info("not placing offer because we run the risk of running out of lumens | xlmExposure: ", xlmExposure,
				" | incrementalXlmAmount: ", incrementalXlmAmount, " | bal: ", bal, " | minAccountBal: ", minAccountBal,
				" | operationalBuffer: ", operationalBuffer)
			return nil
		}
	}

	stringPrice := strconv.FormatFloat(float64(price), 'f', 8, 64)
	rate := build.Rate{
		Selling: Asset2Asset(selling),
		Buying:  Asset2Asset(buying),
		Price:   build.Price(stringPrice),
	}

	//log.Info("sa: ", self.sourceAccount, " ta:", self.tradingAccount, " r:", rate)
	mutators := []interface{}{
		rate,
		build.Amount(strconv.FormatFloat(float64(amount), 'f', -1, 64)),
	}
	if offer != nil {
		mutators = append(mutators, build.OfferID(offer.ID))
	}
	if self.SourceAccount != self.TradingAccount {
		mutators = append(mutators, build.SourceAccount{AddressOrSeed: self.TradingAccount})
	}
	result := build.ManageOffer(false, mutators...)
	return &result
}

func (self *TxButler) SubmitOps(ops []build.TransactionMutator) {
	self.incrementSeqNum()
	muts := []build.TransactionMutator{
		build.Sequence{self.seqNum},
		self.Network,
		build.SourceAccount{self.SourceAccount},
	}
	muts = append(muts, ops...)
	tx := build.Transaction(muts...)
	go self.signAndSubmit(tx)
}

func (self *TxButler) CreateBuyOffer(base horizon.Asset, counter horizon.Asset, price float64, amount float64) *build.ManageOfferBuilder {
	//log.Info("createBuyOffer: ", price, amount)
	return self.CreateSellOffer(counter, base, 1/price, amount*price)
}

func (self *TxButler) signAndSubmit(tx *build.TransactionBuilder) {

	if tx.Err != nil {
		log.Info("s&s err ", tx.Err)
		return
	}

	var txe build.TransactionEnvelopeBuilder
	if self.SourceSeed != self.TradingSeed {
		txe = tx.Sign(self.SourceSeed, self.TradingSeed)
	} else {
		txe = tx.Sign(self.SourceSeed)
	}

	txeB64, err := txe.Base64()
	if err != nil {
		log.Error("", err)
		return
	}
	log.Info("tx: ", txeB64)

	resp, err := self.API.SubmitTransaction(txeB64)
	if err != nil {
		if herr, ok := errors.Cause(err).(*horizon.Error); ok {
			rcs, err := herr.ResultCodes()
			if err != nil {
				log.Info("no rc from horizon: ", err)
				return
			}
			if rcs.TransactionCode == "tx_bad_seq" {
				log.Info("tx_bad_seq, reloading")
				self.reloadSeqNum = true
			}

			log.Info("tx code: ", rcs.TransactionCode, " opcodes: ", rcs.OperationCodes)

		} else {
			log.Info("tx failed: ", err)
		}

		return
	}
	log.Info("", resp.Hash)
}

// ResetCachedXlmExposure resets the cache
func (t *TxButler) ResetCachedXlmExposure() {
	t.cachedXlmExposure = nil
}

func (self *TxButler) xlmExposure() (float64, error) {
	if self.cachedXlmExposure != nil {
		return *self.cachedXlmExposure, nil
	}

	// uses all offers for this trading account to accommodate sharing by other bots
	offers, err := LoadAllOffers(self.TradingAccount, self.API)
	if err != nil {
		log.Info("error computing XLM exposure: ", err)
		return -1, err
	}

	var sum float64
	for _, offer := range offers {
		// only need to compute sum of selling because that's the max XLM we can give up if all our offers are taken
		if offer.Selling.Type == "native" {
			offerAmt, err := self.ParseOfferAmount(offer.Amount)
			if err != nil {
				return -1, err
			}
			sum += offerAmt
		}
	}

	self.cachedXlmExposure = &sum
	return sum, nil
}
