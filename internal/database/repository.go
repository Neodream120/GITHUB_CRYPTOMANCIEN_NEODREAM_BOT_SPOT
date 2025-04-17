// internal/database/repository.go
package database

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ostafen/clover"
)

// CycleRepository gère les opérations de base de données pour les cycles
type CycleRepository struct {
	db *clover.DB
	mu sync.Mutex
}

// FindAll retourne tous les cycles
func (r *CycleRepository) FindAll() ([]*Cycle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.db == nil {
		return nil, fmt.Errorf("la base de données n'est pas initialisée")
	}

	docs, err := r.db.Query(CollectionName).Sort(clover.SortOption{
		Field:     "idInt",
		Direction: -1,
	}).FindAll()

	if err != nil {
		return nil, err
	}

	cycles := make([]*Cycle, 0, len(docs))
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

		// Récupérer la date de complétion si elle existe
		var completedAt time.Time
		if completedAtValue := doc.Get("completedAt"); completedAtValue != nil {
			if timeStr, ok := completedAtValue.(string); ok && timeStr != "" {
				parsedTime, err := time.Parse(time.RFC3339, timeStr)
				if err == nil {
					completedAt = parsedTime
				}
			}
		}

		cycle := &Cycle{
			IdInt:       int32(doc.Get("idInt").(int64)),
			Exchange:    doc.Get("exchange").(string),
			Status:      doc.Get("status").(string),
			Quantity:    doc.Get("quantity").(float64),
			BuyPrice:    doc.Get("buyPrice").(float64),
			BuyId:       doc.Get("buyId").(string),
			SellPrice:   doc.Get("sellPrice").(float64),
			SellId:      doc.Get("sellId").(string),
			CreatedAt:   createdAt,
			CompletedAt: completedAt,
		}
		cycles = append(cycles, cycle)
	}

	return cycles, nil
}

// FindById récupère un cycle par son ID
func (r *CycleRepository) FindById(id string) (*Cycle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.db == nil {
		return nil, fmt.Errorf("la base de données n'est pas initialisée")
	}

	doc, err := r.db.Query(CollectionName).FindById(id)
	if err != nil {
		return nil, err
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

	// Récupérer la date de complétion si elle existe
	var completedAt time.Time
	if completedAtValue := doc.Get("completedAt"); completedAtValue != nil {
		if timeStr, ok := completedAtValue.(string); ok && timeStr != "" {
			parsedTime, err := time.Parse(time.RFC3339, timeStr)
			if err == nil {
				completedAt = parsedTime
			}
		}
	}

	cycle := &Cycle{
		IdInt:       int32(doc.Get("idInt").(int64)),
		Exchange:    doc.Get("exchange").(string),
		Status:      doc.Get("status").(string),
		Quantity:    doc.Get("quantity").(float64),
		BuyPrice:    doc.Get("buyPrice").(float64),
		BuyId:       doc.Get("buyId").(string),
		SellPrice:   doc.Get("sellPrice").(float64),
		SellId:      doc.Get("sellId").(string),
		CreatedAt:   createdAt,
		CompletedAt: completedAt, // Ajout du nouveau champ
	}

	return cycle, nil
}

// FindByIdInt récupère un cycle par son ID entier
func (r *CycleRepository) FindByIdInt(id int32) (*Cycle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.db == nil {
		return nil, fmt.Errorf("la base de données n'est pas initialisée")
	}

	doc, err := r.db.Query(CollectionName).Where(clover.Field("idInt").Eq(id)).FindFirst()
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

	// Récupérer la date de complétion si elle existe
	var completedAt time.Time
	if completedAtValue := doc.Get("completedAt"); completedAtValue != nil {
		if timeStr, ok := completedAtValue.(string); ok && timeStr != "" {
			parsedTime, err := time.Parse(time.RFC3339, timeStr)
			if err == nil {
				completedAt = parsedTime
			}
		}
	}

	cycle := &Cycle{
		IdInt:       int32(doc.Get("idInt").(int64)),
		Exchange:    doc.Get("exchange").(string),
		Status:      doc.Get("status").(string),
		Quantity:    doc.Get("quantity").(float64),
		BuyPrice:    doc.Get("buyPrice").(float64),
		BuyId:       doc.Get("buyId").(string),
		SellPrice:   doc.Get("sellPrice").(float64),
		SellId:      doc.Get("sellId").(string),
		CreatedAt:   createdAt,
		CompletedAt: completedAt, // Ajout du nouveau champ
	}

	return cycle, nil
}

func (r *CycleRepository) Save(cycle *Cycle) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.db == nil {
		return "", fmt.Errorf("la base de données n'est pas initialisée")
	}

	// Vérifier si c'est un nouveau cycle (il faut générer un ID)
	if cycle.IdInt == 0 {
		cycle.IdInt = r.getNextId()

		// Initialiser la date de création pour les nouveaux cycles
		if cycle.CreatedAt.IsZero() {
			cycle.CreatedAt = time.Now()
		}
	}

	doc := clover.NewDocument()
	doc.Set("idInt", cycle.IdInt)
	doc.Set("exchange", cycle.Exchange)
	doc.Set("status", cycle.Status)
	doc.Set("quantity", cycle.Quantity)
	doc.Set("buyPrice", cycle.BuyPrice)
	doc.Set("buyId", cycle.BuyId)
	doc.Set("sellPrice", cycle.SellPrice)
	doc.Set("sellId", cycle.SellId)
	doc.Set("createdAt", cycle.CreatedAt.Format(time.RFC3339))

	// Ajouter la date de complétion si elle existe
	if !cycle.CompletedAt.IsZero() {
		doc.Set("completedAt", cycle.CompletedAt.Format(time.RFC3339))
	} else {
		doc.Set("completedAt", "")
	}

	docId, err := r.db.InsertOne(CollectionName, doc)
	if err != nil {
		return "", fmt.Errorf("erreur lors de l'insertion du document: %v", err)
	}

	return docId, nil
}

// Update met à jour un champ spécifique d'un cycle
func (r *CycleRepository) Update(id string, field string, value interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.db == nil {
		return fmt.Errorf("la base de données n'est pas initialisée")
	}

	return r.db.Query(CollectionName).UpdateById(id, map[string]interface{}{field: value})
}

// UpdateByIdInt met à jour un cycle par son ID entier
func (r *CycleRepository) UpdateByIdInt(idInt int32, updates map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.db == nil {
		return fmt.Errorf("la base de données n'est pas initialisée")
	}

	return r.db.Query(CollectionName).
		Where(clover.Field("idInt").Eq(idInt)).
		Update(updates)
}

// Delete supprime un cycle par son ID
func (r *CycleRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.db == nil {
		return fmt.Errorf("la base de données n'est pas initialisée")
	}

	return r.db.Query(CollectionName).DeleteById(id)
}

// DeleteByIdInt supprime un cycle par son ID entier
func (r *CycleRepository) DeleteByIdInt(idInt int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fmt.Printf("Tentative de suppression du cycle %d\n", idInt)

	if r.db == nil {
		return fmt.Errorf("la base de données n'est pas initialisée")
	}

	err := r.db.Query(CollectionName).
		Where(clover.Field("idInt").Eq(idInt)).
		Delete()

	if err != nil {
		fmt.Printf("Erreur lors de la suppression du cycle %d: %v\n", idInt, err)
	} else {
		fmt.Printf("Cycle %d supprimé avec succès\n", idInt)
	}

	return err
}

// ListPaginated récupère une liste paginée de cycles
func (r *CycleRepository) ListPaginated(page, perPage int) ([]*Cycle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.db == nil {
		return nil, fmt.Errorf("la base de données n'est pas initialisée")
	}

	skip := (page - 1) * perPage
	docs, err := r.db.Query(CollectionName).
		Sort(clover.SortOption{Field: "idInt", Direction: -1}).
		Skip(skip).
		Limit(perPage).
		FindAll()

	if err != nil {
		return nil, err
	}

	cycles := make([]*Cycle, 0, len(docs))
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

		cycle := &Cycle{
			IdInt:     int32(doc.Get("idInt").(int64)),
			Exchange:  doc.Get("exchange").(string),
			Status:    doc.Get("status").(string),
			Quantity:  doc.Get("quantity").(float64),
			BuyPrice:  doc.Get("buyPrice").(float64),
			BuyId:     doc.Get("buyId").(string),
			SellPrice: doc.Get("sellPrice").(float64),
			SellId:    doc.Get("sellId").(string),
			CreatedAt: createdAt,
		}
		cycles = append(cycles, cycle)
	}

	return cycles, nil
}

// getNextId génère un nouvel ID pour un cycle
func (r *CycleRepository) getNextId() int32 {
	if r.db == nil {
		log.Printf("Base de données non initialisée lors de la génération d'ID")
		return 1
	}

	count, err := r.db.Query(CollectionName).Count()
	if err != nil {
		log.Printf("Erreur lors du comptage des documents: %v", err)
		return 1
	}

	if count == 0 {
		return 1
	}

	lastDoc, err := r.db.Query(CollectionName).
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

// CountByStatus compte les cycles par statut
func (r *CycleRepository) CountByStatus(status string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.db == nil {
		return 0, fmt.Errorf("la base de données n'est pas initialisée")
	}

	count, err := r.db.Query(CollectionName).
		Where(clover.Field("status").Eq(status)).
		Count()

	return count, err
}

// GetStatistics récupère des statistiques sur les cycles
func (r *CycleRepository) GetStatistics() (map[string]interface{}, error) {
	cycles, err := r.FindAll()
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"totalCycles":     len(cycles),
		"completedCycles": 0,
		"buyCycles":       0,
		"sellCycles":      0,
		"totalBuy":        0.0,
		"totalSell":       0.0,
		"gainAbsolute":    0.0,
		"gainPercent":     0.0,
	}

	for _, cycle := range cycles {
		switch cycle.Status {
		case "completed":
			stats["completedCycles"] = stats["completedCycles"].(int) + 1
			buyValue := cycle.BuyPrice * cycle.Quantity
			sellValue := cycle.SellPrice * cycle.Quantity
			stats["totalBuy"] = stats["totalBuy"].(float64) + buyValue
			stats["totalSell"] = stats["totalSell"].(float64) + sellValue
		case "buy":
			stats["buyCycles"] = stats["buyCycles"].(int) + 1
		case "sell":
			stats["sellCycles"] = stats["sellCycles"].(int) + 1
		}
	}

	totalBuy := stats["totalBuy"].(float64)
	totalSell := stats["totalSell"].(float64)

	stats["gainAbsolute"] = totalSell - totalBuy

	if totalBuy > 0 {
		stats["gainPercent"] = (totalSell - totalBuy) / totalBuy * 100
	}

	return stats, nil
}
