// internal/database/accumulation.go
package database

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ostafen/clover"
)

const AccumulationCollectionName = "accumulations"

// Accumulation représente un ordre de vente annulé pour accumulation
type Accumulation struct {
	IdInt            int32     `json:"idInt"`            // ID unique
	Exchange         string    `json:"exchange"`         // Nom de l'exchange
	CycleIdInt       int32     `json:"cycleIdInt"`       // ID du cycle associé
	Quantity         float64   `json:"quantity"`         // Quantité de BTC accumulée
	OriginalBuyPrice float64   `json:"originalBuyPrice"` // Prix d'achat original
	TargetSellPrice  float64   `json:"targetSellPrice"`  // Prix de vente original qui a été annulé
	CancelPrice      float64   `json:"cancelPrice"`      // Prix du BTC au moment de l'annulation
	Deviation        float64   `json:"deviation"`        // Déviation en pourcentage qui a déclenché l'accumulation
	CreatedAt        time.Time `json:"createdAt"`        // Date de création de l'accumulation
}

// AccumulationRepository gère les opérations de base de données pour les accumulations
type AccumulationRepository struct {
	db *clover.DB
	mu sync.Mutex
}

// FindAll retourne toutes les accumulations
func (r *AccumulationRepository) FindAll() ([]*Accumulation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	docs, err := r.db.Query(AccumulationCollectionName).Sort(clover.SortOption{
		Field:     "idInt",
		Direction: -1,
	}).FindAll()

	if err != nil {
		return nil, err
	}

	accumulations := make([]*Accumulation, 0, len(docs))
	for _, doc := range docs {
		// Récupérer la date de création si elle existe
		var createdAt time.Time
		if createdAtValue := doc.Get("createdAt"); createdAtValue != nil {
			if timeStr, ok := createdAtValue.(string); ok {
				parsedTime, err := time.Parse(time.RFC3339, timeStr)
				if err == nil {
					createdAt = parsedTime
				}
			}
		}

		accumulation := &Accumulation{
			IdInt:            int32(doc.Get("idInt").(int64)),
			Exchange:         doc.Get("exchange").(string),
			CycleIdInt:       int32(doc.Get("cycleIdInt").(int64)),
			Quantity:         doc.Get("quantity").(float64),
			OriginalBuyPrice: doc.Get("originalBuyPrice").(float64),
			TargetSellPrice:  doc.Get("targetSellPrice").(float64),
			CancelPrice:      doc.Get("cancelPrice").(float64),
			Deviation:        doc.Get("deviation").(float64),
			CreatedAt:        createdAt,
		}
		accumulations = append(accumulations, accumulation)
	}

	return accumulations, nil
}

// FindByExchange retourne toutes les accumulations pour un exchange spécifique
func (r *AccumulationRepository) FindByExchange(exchange string) ([]*Accumulation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	docs, err := r.db.Query(AccumulationCollectionName).
		Where(clover.Field("exchange").Eq(exchange)).
		Sort(clover.SortOption{Field: "idInt", Direction: -1}).
		FindAll()

	if err != nil {
		return nil, err
	}

	accumulations := make([]*Accumulation, 0, len(docs))
	for _, doc := range docs {
		// Récupérer la date de création si elle existe
		var createdAt time.Time
		if createdAtValue := doc.Get("createdAt"); createdAtValue != nil {
			if timeStr, ok := createdAtValue.(string); ok {
				parsedTime, err := time.Parse(time.RFC3339, timeStr)
				if err == nil {
					createdAt = parsedTime
				}
			}
		}

		accumulation := &Accumulation{
			IdInt:            int32(doc.Get("idInt").(int64)),
			Exchange:         doc.Get("exchange").(string),
			CycleIdInt:       int32(doc.Get("cycleIdInt").(int64)),
			Quantity:         doc.Get("quantity").(float64),
			OriginalBuyPrice: doc.Get("originalBuyPrice").(float64),
			TargetSellPrice:  doc.Get("targetSellPrice").(float64),
			CancelPrice:      doc.Get("cancelPrice").(float64),
			Deviation:        doc.Get("deviation").(float64),
			CreatedAt:        createdAt,
		}
		accumulations = append(accumulations, accumulation)
	}

	return accumulations, nil
}

// FindByIdInt récupère une accumulation par son ID entier
func (r *AccumulationRepository) FindByIdInt(id int32) (*Accumulation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.db.Query(AccumulationCollectionName).Where(clover.Field("idInt").Eq(id)).FindFirst()
	if err != nil {
		return nil, err
	}

	// Si aucun document n'est trouvé
	if doc == nil {
		return nil, nil
	}

	// Récupérer la date de création si elle existe
	var createdAt time.Time
	if createdAtValue := doc.Get("createdAt"); createdAtValue != nil {
		if timeStr, ok := createdAtValue.(string); ok {
			parsedTime, err := time.Parse(time.RFC3339, timeStr)
			if err == nil {
				createdAt = parsedTime
			}
		}
	}

	accumulation := &Accumulation{
		IdInt:            int32(doc.Get("idInt").(int64)),
		Exchange:         doc.Get("exchange").(string),
		CycleIdInt:       int32(doc.Get("cycleIdInt").(int64)),
		Quantity:         doc.Get("quantity").(float64),
		OriginalBuyPrice: doc.Get("originalBuyPrice").(float64),
		TargetSellPrice:  doc.Get("targetSellPrice").(float64),
		CancelPrice:      doc.Get("cancelPrice").(float64),
		Deviation:        doc.Get("deviation").(float64),
		CreatedAt:        createdAt,
	}

	return accumulation, nil
}

// Save enregistre une accumulation dans la base de données
func (r *AccumulationRepository) Save(accumulation *Accumulation) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Vérifier si c'est une nouvelle accumulation (il faut générer un ID)
	if accumulation.IdInt == 0 {
		accumulation.IdInt = r.getNextId()

		// Initialiser la date de création pour les nouvelles accumulations
		if accumulation.CreatedAt.IsZero() {
			accumulation.CreatedAt = time.Now()
		}
	}

	doc := clover.NewDocument()
	doc.Set("idInt", accumulation.IdInt)
	doc.Set("exchange", accumulation.Exchange)
	doc.Set("cycleIdInt", accumulation.CycleIdInt)
	doc.Set("quantity", accumulation.Quantity)
	doc.Set("originalBuyPrice", accumulation.OriginalBuyPrice)
	doc.Set("targetSellPrice", accumulation.TargetSellPrice)
	doc.Set("cancelPrice", accumulation.CancelPrice)
	doc.Set("deviation", accumulation.Deviation)
	doc.Set("createdAt", accumulation.CreatedAt.Format(time.RFC3339))

	docId, err := r.db.InsertOne(AccumulationCollectionName, doc)
	if err != nil {
		return "", fmt.Errorf("erreur lors de l'insertion du document: %v", err)
	}

	return docId, nil
}

// DeleteByIdInt supprime une accumulation par son ID entier
func (r *AccumulationRepository) DeleteByIdInt(idInt int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.db.Query(AccumulationCollectionName).
		Where(clover.Field("idInt").Eq(idInt)).
		Delete()
}

// CountByExchange compte les accumulations par exchange
func (r *AccumulationRepository) CountByExchange(exchange string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	count, err := r.db.Query(AccumulationCollectionName).
		Where(clover.Field("exchange").Eq(exchange)).
		Count()

	return count, err
}

// GetTotalAccumulatedBTC retourne le total de BTC accumulé pour un exchange
func (r *AccumulationRepository) GetTotalAccumulatedBTC(exchange string) (float64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	docs, err := r.db.Query(AccumulationCollectionName).
		Where(clover.Field("exchange").Eq(exchange)).
		FindAll()

	if err != nil {
		return 0, err
	}

	var totalBTC float64
	for _, doc := range docs {
		quantity := doc.Get("quantity").(float64)
		totalBTC += quantity
	}

	return totalBTC, nil
}

// GetTotalAccumulatedValue retourne la valeur totale accumulée pour un exchange
func (r *AccumulationRepository) GetTotalAccumulatedValue(exchange string) (float64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	docs, err := r.db.Query(AccumulationCollectionName).
		Where(clover.Field("exchange").Eq(exchange)).
		FindAll()

	if err != nil {
		return 0, err
	}

	var totalValue float64
	for _, doc := range docs {
		quantity := doc.Get("quantity").(float64)
		targetSellPrice := doc.Get("targetSellPrice").(float64)
		totalValue += quantity * targetSellPrice
	}

	return totalValue, nil
}

// getNextId génère un nouvel ID pour une accumulation
func (r *AccumulationRepository) getNextId() int32 {
	count, err := r.db.Query(AccumulationCollectionName).Count()
	if err != nil {
		log.Printf("Erreur lors du comptage des documents: %v", err)
		return 1
	}

	if count == 0 {
		return 1
	}

	lastDoc, err := r.db.Query(AccumulationCollectionName).
		Sort(clover.SortOption{Field: "idInt", Direction: -1}).
		Limit(1).
		FindFirst()

	if err != nil || lastDoc == nil {
		log.Printf("Erreur lors de la récupération du dernier document: %v", err)
		return 1
	}

	lastId := lastDoc.Get("idInt").(int64)
	nextId := lastId + 1

	return int32(nextId)
}

// Fonction pour obtenir les statistiques des accumulations par exchange
func (r *AccumulationRepository) GetExchangeAccumulationStats(exchange string) (map[string]interface{}, error) {
	accumulations, err := r.FindByExchange(exchange)
	if err != nil {
		return nil, err
	}

	totalQuantity := 0.0
	totalOriginalValue := 0.0
	totalCancelValue := 0.0
	averageDeviation := 0.0

	if len(accumulations) > 0 {
		for _, acc := range accumulations {
			totalQuantity += acc.Quantity
			totalOriginalValue += acc.Quantity * acc.TargetSellPrice
			totalCancelValue += acc.Quantity * acc.CancelPrice
			averageDeviation += acc.Deviation
		}
		averageDeviation /= float64(len(accumulations))
	}

	stats := map[string]interface{}{
		"count":              len(accumulations),
		"totalQuantity":      totalQuantity,
		"totalOriginalValue": totalOriginalValue,
		"totalCancelValue":   totalCancelValue,
		"savedValue":         totalOriginalValue - totalCancelValue,
		"averageDeviation":   averageDeviation,
	}

	return stats, nil
}
