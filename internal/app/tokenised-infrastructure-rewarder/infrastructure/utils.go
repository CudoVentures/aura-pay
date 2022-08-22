package infrastructure

import (
	"bytes"
	"encoding/hex"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

func CreateTransaction(secret string, destination string, amount int64, txHash string) (types.Transaction, error) {

	wif, err := LoadPrivateKey(secret)
	if err != nil {
		return types.Transaction{}, err
	}

	addresspubkey, _ := btcutil.NewAddressPubKey(wif.PrivKey.PubKey().SerializeUncompressed(), &chaincfg.SigNetParams)
	sourceAddress, err := btcutil.DecodeAddress(addresspubkey.EncodeAddress(), &chaincfg.SigNetParams)

	destinationAddress, err := btcutil.DecodeAddress(destination, &chaincfg.SigNetParams) // TODO add debug mode
	if err != nil {
		return types.Transaction{}, err
	}

	sourceTx, err := CreateSourceTx(wif, destination, amount, txHash, sourceAddress)
	if err != nil {
		return types.Transaction{}, err
	}
	sourceTxHash := sourceTx.TxHash()

	redeemTx, err := CreateRedeemTx(sourceTxHash, destination, amount, destinationAddress)
	err = SignTx(sourceTx, redeemTx, wif)
	if err != nil {
		return types.Transaction{}, err
	}
	err = CheckSignatureIsValid(sourceTx, redeemTx, amount)
	if err != nil {
		return types.Transaction{}, err
	}

	var transaction types.Transaction
	var unsignedTx bytes.Buffer
	var signedTx bytes.Buffer
	sourceTx.Serialize(&unsignedTx)
	redeemTx.Serialize(&signedTx)
	transaction.TxId = sourceTxHash.String()
	transaction.UnsignedTx = hex.EncodeToString(unsignedTx.Bytes())
	transaction.Amount = amount
	transaction.SignedTx = hex.EncodeToString(signedTx.Bytes())
	transaction.SourceAddress = sourceAddress.EncodeAddress()
	transaction.DestinationAddress = destinationAddress.EncodeAddress()
	return transaction, nil

}

func CheckSignatureIsValid(sourceTx *wire.MsgTx, redeemTx *wire.MsgTx, amount int64) error {
	flags := txscript.StandardVerifyFlags
	vm, err := txscript.NewEngine(sourceTx.TxOut[0].PkScript, redeemTx, 0, flags, nil, nil, amount, nil)

	if err != nil {
		return err
	}
	if err := vm.Execute(); err != nil {
		return err
	}

	return nil
}

func SignTx(sourceTx *wire.MsgTx, redeemTx *wire.MsgTx, wif *btcutil.WIF) error {
	sigScript, err := txscript.SignatureScript(redeemTx, 0, sourceTx.TxOut[0].PkScript, txscript.SigHashAll, wif.PrivKey, false)
	if err != nil {
		return err
	}
	redeemTx.TxIn[0].SignatureScript = sigScript
	return nil
}

func CreateRedeemTx(sourceTxHash chainhash.Hash, destination string, amount int64, destinationAddress btcutil.Address) (*wire.MsgTx, error) {

	destinationPkScript, _ := txscript.PayToAddrScript(destinationAddress)
	redeemTx := wire.NewMsgTx(wire.TxVersion)
	prevOut := wire.NewOutPoint(&sourceTxHash, 0)
	redeemTxIn := wire.NewTxIn(prevOut, nil, nil)
	redeemTx.AddTxIn(redeemTxIn)
	redeemTxOut := wire.NewTxOut(amount, destinationPkScript)
	redeemTx.AddTxOut(redeemTxOut)
	return redeemTx, nil
}

func CreateSourceTx(wif *btcutil.WIF, destination string, amount int64, txHash string, sourceAddress btcutil.Address) (*wire.MsgTx, error) {
	sourceTx := wire.NewMsgTx(wire.TxVersion)
	sourceUtxoHash, err := chainhash.NewHashFromStr(txHash)
	if err != nil {
		return nil, err
	}
	sourceUtxo := wire.NewOutPoint(sourceUtxoHash, 0)
	sourceTxIn := wire.NewTxIn(sourceUtxo, nil, nil)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	sourcePkScript, _ := txscript.PayToAddrScript(sourceAddress)
	sourceTxOut := wire.NewTxOut(amount, sourcePkScript)
	sourceTx.AddTxIn(sourceTxIn)
	sourceTx.AddTxOut(sourceTxOut)
	return sourceTx, nil
}

func LoadPrivateKey(secret string) (*btcutil.WIF, error) {
	wif, err := btcutil.DecodeWIF(secret)
	if err != nil {
		return nil, err
	}
	return wif, nil
}
