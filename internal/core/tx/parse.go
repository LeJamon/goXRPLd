package tx

import (
	"encoding/hex"
	"encoding/json"
	"errors"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
)

// ParseJSON parses a JSON transaction into the appropriate transaction type
func ParseJSON(data []byte) (Transaction, error) {
	// First, parse to get the transaction type
	var header struct {
		TransactionType string `json:"TransactionType"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, errors.New("failed to parse transaction: " + err.Error())
	}

	if header.TransactionType == "" {
		return nil, ErrInvalidTransactionType
	}

	// Get the type from string
	txType, err := TypeFromString(header.TransactionType)
	if err != nil {
		return nil, err
	}

	// Parse into the specific transaction type
	switch txType {
	case TypePayment:
		var tx Payment
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse Payment: " + err.Error())
		}
		tx.txType = TypePayment
		return &tx, nil

	case TypeAccountSet:
		var tx AccountSet
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse AccountSet: " + err.Error())
		}
		tx.txType = TypeAccountSet
		return &tx, nil

	case TypeTrustSet:
		var tx TrustSet
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse TrustSet: " + err.Error())
		}
		tx.txType = TypeTrustSet
		return &tx, nil

	case TypeOfferCreate:
		var tx OfferCreate
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse OfferCreate: " + err.Error())
		}
		tx.txType = TypeOfferCreate
		return &tx, nil

	case TypeOfferCancel:
		var tx OfferCancel
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse OfferCancel: " + err.Error())
		}
		tx.txType = TypeOfferCancel
		return &tx, nil

	case TypeRegularKeySet:
		var tx SetRegularKey
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse SetRegularKey: " + err.Error())
		}
		tx.txType = TypeRegularKeySet
		return &tx, nil

	case TypeSignerListSet:
		var tx SignerListSet
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse SignerListSet: " + err.Error())
		}
		tx.txType = TypeSignerListSet
		return &tx, nil

	case TypeEscrowCreate:
		var tx EscrowCreate
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse EscrowCreate: " + err.Error())
		}
		tx.txType = TypeEscrowCreate
		return &tx, nil

	case TypeEscrowFinish:
		var tx EscrowFinish
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse EscrowFinish: " + err.Error())
		}
		tx.txType = TypeEscrowFinish
		return &tx, nil

	case TypeEscrowCancel:
		var tx EscrowCancel
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse EscrowCancel: " + err.Error())
		}
		tx.txType = TypeEscrowCancel
		return &tx, nil

	case TypePaymentChannelCreate:
		var tx PaymentChannelCreate
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse PaymentChannelCreate: " + err.Error())
		}
		tx.txType = TypePaymentChannelCreate
		return &tx, nil

	case TypePaymentChannelFund:
		var tx PaymentChannelFund
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse PaymentChannelFund: " + err.Error())
		}
		tx.txType = TypePaymentChannelFund
		return &tx, nil

	case TypePaymentChannelClaim:
		var tx PaymentChannelClaim
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse PaymentChannelClaim: " + err.Error())
		}
		tx.txType = TypePaymentChannelClaim
		return &tx, nil

	case TypeCheckCreate:
		var tx CheckCreate
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse CheckCreate: " + err.Error())
		}
		tx.txType = TypeCheckCreate
		return &tx, nil

	case TypeCheckCash:
		var tx CheckCash
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse CheckCash: " + err.Error())
		}
		tx.txType = TypeCheckCash
		return &tx, nil

	case TypeCheckCancel:
		var tx CheckCancel
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse CheckCancel: " + err.Error())
		}
		tx.txType = TypeCheckCancel
		return &tx, nil

	case TypeDepositPreauth:
		var tx DepositPreauth
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse DepositPreauth: " + err.Error())
		}
		tx.txType = TypeDepositPreauth
		return &tx, nil

	case TypeTicketCreate:
		var tx TicketCreate
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse TicketCreate: " + err.Error())
		}
		tx.txType = TypeTicketCreate
		return &tx, nil

	default:
		// For unimplemented types, parse as generic BaseTx
		var tx BaseTx
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, errors.New("failed to parse transaction: " + err.Error())
		}
		tx.txType = txType
		return &tx, nil
	}
}

// TypeFromString converts a transaction type string to a Type
func TypeFromString(s string) (Type, error) {
	t, ok := TypeFromName(s)
	if !ok {
		return 0, ErrInvalidTransactionType
	}
	return t, nil
}

// ParseFromBinary parses a binary transaction blob into a Transaction
func ParseFromBinary(blob []byte) (Transaction, error) {
	// Convert binary to hex string for the codec
	hexStr := hex.EncodeToString(blob)

	// Decode binary to JSON map
	jsonMap, err := binarycodec.Decode(hexStr)
	if err != nil {
		return nil, errors.New("failed to decode binary transaction: " + err.Error())
	}

	// Extract present fields from the decoded map
	// This is used to distinguish between absent fields and empty values
	presentFields := make(map[string]bool)
	for key := range jsonMap {
		presentFields[key] = true
	}

	// Convert map to JSON bytes
	jsonBytes, err := json.Marshal(jsonMap)
	if err != nil {
		return nil, errors.New("failed to marshal decoded transaction: " + err.Error())
	}

	// Parse the JSON into a transaction
	tx, err := ParseJSON(jsonBytes)
	if err != nil {
		return nil, err
	}

	// Set the present fields on the parsed transaction
	tx.GetCommon().SetPresentFields(presentFields)

	return tx, nil
}
