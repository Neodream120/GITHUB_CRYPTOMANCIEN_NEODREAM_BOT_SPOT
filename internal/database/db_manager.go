// internal/database/db_manager.go
package database

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ostafen/clover"
)

var (
	repositoryInstance       *CycleRepository
	accumulationRepoInstance *AccumulationRepository
	initOnce                 sync.Once
	db                       *clover.DB
)

// InitDatabase initialise la base de données
func InitDatabase() {
	initOnce.Do(func() {
		// Obtenir le chemin de la base de données
		dbPath := GetDatabasePath()

		// Vérifier et supprimer le fichier LOCK s'il existe
		lockFile := filepath.Join(dbPath, "LOCK")
		if _, err := os.Stat(lockFile); err == nil {
			log.Printf("Ancien fichier LOCK détecté. Tentative de suppression...")
			os.Remove(lockFile)
		}

		// Ouvrir la base de données
		var err error
		db, err = clover.Open(dbPath)
		if err != nil {
			log.Fatalf("Erreur lors de l'ouverture de la base de données: %v", err)
		}

		// Créer les collections si elles n'existent pas
		ensureCollectionsExist()

		// Nettoyer la base de données au démarrage
		CleanupDatabase()
	})
}

// ensureCollectionsExist s'assure que toutes les collections nécessaires existent
func ensureCollectionsExist() {
	// Vérifier la collection pour les cycles
	collectionExists, err := db.HasCollection(CollectionName)
	if err != nil {
		log.Fatalf("Erreur lors de la vérification de la collection: %v", err)
	}

	if !collectionExists {
		err = db.CreateCollection(CollectionName)
		if err != nil {
			log.Fatalf("Erreur lors de la création de la collection: %v", err)
		}
		log.Printf("Collection %s créée avec succès", CollectionName)
	}

	// Vérifier la collection pour les accumulations
	accuCollectionExists, err := db.HasCollection(AccumulationCollectionName)
	if err != nil {
		log.Fatalf("Erreur lors de la vérification de la collection d'accumulation: %v", err)
	}

	if !accuCollectionExists {
		err = db.CreateCollection(AccumulationCollectionName)
		if err != nil {
			log.Fatalf("Erreur lors de la création de la collection d'accumulation: %v", err)
		}
		log.Printf("Collection %s créée avec succès", AccumulationCollectionName)
	}
}

// GetRepository retourne l'instance du repository de cycles
func GetRepository() *CycleRepository {
	if repositoryInstance == nil {
		repositoryInstance = &CycleRepository{
			db: db,
		}
	}
	return repositoryInstance
}

// GetAccumulationRepository retourne l'instance du repository d'accumulation
func GetAccumulationRepository() *AccumulationRepository {
	if accumulationRepoInstance == nil {
		accumulationRepoInstance = &AccumulationRepository{
			db: db,
		}
	}
	return accumulationRepoInstance
}

// CloseDatabase ferme proprement la connexion à la base de données
func CloseDatabase() {
	if db != nil {
		if err := db.Close(); err != nil {
			log.Printf("Erreur lors de la fermeture de la base de données: %v", err)
		}
		db = nil
		repositoryInstance = nil
		accumulationRepoInstance = nil
	}
}

func CleanupDatabase() {
	if db == nil {
		log.Println("La base de données n'est pas initialisée")
		return
	}

	log.Println("Démarrage du nettoyage de la base de données...")

	// Récupérer tous les cycles
	repo := GetRepository()
	cycles, err := repo.FindAll()
	if err != nil {
		log.Printf("Erreur lors de la récupération des cycles: %v", err)
		return
	}

	cleanupCount := 0

	// Parcourir chaque cycle
	for _, cycle := range cycles {
		// Vérifier les cycles "buy" et "sell" sans ID d'ordre valide
		if cycle.Status == "buy" && (cycle.BuyId == "" || strings.TrimSpace(cycle.BuyId) == "") {
			log.Printf("Cycle %d: Statut 'buy' sans ID d'ordre valide, suppression...", cycle.IdInt)
			err := repo.DeleteByIdInt(cycle.IdInt)
			if err != nil {
				log.Printf("Erreur lors de la suppression du cycle %d: %v", cycle.IdInt, err)
			} else {
				cleanupCount++
			}
			continue
		}

		if cycle.Status == "sell" && (cycle.SellId == "" || strings.TrimSpace(cycle.SellId) == "") {
			log.Printf("Cycle %d: Statut 'sell' sans ID d'ordre valide, suppression...", cycle.IdInt)
			err := repo.DeleteByIdInt(cycle.IdInt)
			if err != nil {
				log.Printf("Erreur lors de la suppression du cycle %d: %v", cycle.IdInt, err)
			} else {
				cleanupCount++
			}
			continue
		}

		// Vérifier les cycles très anciens (plus de 30 jours)
		if cycle.Status == "buy" || cycle.Status == "sell" {
			if cycle.GetAge() > 30 {
				log.Printf("Cycle %d: Ordre vieux de %.2f jours (> 30 jours), suppression...", cycle.IdInt, cycle.GetAge())
				err := repo.DeleteByIdInt(cycle.IdInt)
				if err != nil {
					log.Printf("Erreur lors de la suppression du cycle %d: %v", cycle.IdInt, err)
				} else {
					cleanupCount++
				}
				continue
			}
		}
	}

	log.Printf("Nettoyage de la base de données terminé. %d cycles ont été nettoyés.", cleanupCount)
}
