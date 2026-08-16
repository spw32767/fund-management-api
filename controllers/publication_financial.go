package controllers

import "errors"

var (
	errFinancialAmountsNonNegative = errors.New("financial amounts must be non-negative")
	errRevisionFeeRequired         = errors.New("revision_fee is required when has_received_reward is true")
	errTotalAmountNonNegative      = errors.New("total amount must be non-negative")
)

func calculatePublicationRequestAmounts(
	hasReceivedReward bool,
	configuredReward float64,
	revisionFee float64,
	publicationFee float64,
	externalFunding float64,
	enforceRequired bool,
) (float64, float64, error) {
	if enforceRequired && (revisionFee < 0 || publicationFee < 0 || externalFunding < 0) {
		return 0, 0, errFinancialAmountsNonNegative
	}

	rewardAmount := configuredReward
	if hasReceivedReward {
		rewardAmount = 0
		if enforceRequired && revisionFee <= 0 {
			return 0, 0, errRevisionFeeRequired
		}
	}

	totalAmount := rewardAmount + revisionFee + publicationFee - externalFunding
	if enforceRequired && totalAmount < 0 {
		return 0, 0, errTotalAmountNonNegative
	}

	return rewardAmount, totalAmount, nil
}

func formatPriorRewardStatus(hasReceivedReward bool) string {
	if hasReceivedReward {
		return "เคยขอเงินรางวัลแล้ว (ไม่คำนวณเงินรางวัล)"
	}
	return "ไม่เคยขอเงินรางวัล"
}
