package common

// DetailedBalance représente les informations détaillées d'un solde d'actif
type DetailedBalance struct {
	Free   float64
	Locked float64
	Total  float64
}

type Exchange interface {
	CheckConnection() error
	GetBalanceUSD() float64
	GetLastPriceBTC() float64
	GetDetailedBalances() (map[string]DetailedBalance, error)
	SetBaseURL(url string)
	CreateOrder(side, price, quantity string) ([]byte, error)
	CreateMakerOrder(side string, price float64, quantity string) ([]byte, error)
	GetOrderById(id string) ([]byte, error)
	IsFilled(id string) bool
	CancelOrder(orderID string) ([]byte, error)
	GetExchangeInfo() ([]byte, error)
	GetAccountInfo() ([]byte, error)
}
