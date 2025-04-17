package commands

import (
	"fmt"
	"main/internal/database"
	"os"
	"strconv"
	"strings"

	"github.com/fatih/color"
)

func Cancel(cancelArg string) {
	// Extraire l'ID du cycle à annuler
	var idStr string

	// Vérifier si l'argument est de la forme "-c=ID" ou "--cancel=ID"
	if strings.HasPrefix(cancelArg, "-c=") || strings.HasPrefix(cancelArg, "--cancel=") {
		parts := strings.Split(cancelArg, "=")
		if len(parts) != 2 {
			color.Red("Format d'ID invalide. Utilisez -c=NOMBRE")
			os.Exit(1)
		}
		idStr = parts[1]
	} else {
		// Si nous sommes ici, c'est un format d'argument invalide
		color.Red("Format d'ID invalide. Utilisez -c=NOMBRE")
		os.Exit(1)
	}

	// Convertir l'ID en nombre entier
	idInt, err := strconv.Atoi(idStr)
	if err != nil {
		color.Red("ID invalide: %s", idStr)
		os.Exit(1)
	}

	color.White("Annulation du cycle %d...", idInt)

	// Récupérer le cycle depuis le repository
	repo := database.GetRepository()
	cycle, err := repo.FindByIdInt(int32(idInt))
	if err != nil {
		color.Red("Erreur lors de la récupération du cycle: %v", err)
		os.Exit(1)
	}

	if cycle == nil {
		color.Red("Cycle avec ID %d introuvable", idInt)
		os.Exit(1)
	}

	// Récupérer les informations du cycle
	status := cycle.Status

	// Obtenir le client de l'échange approprié pour ce cycle
	client := GetClientByExchange(cycle.Exchange)

	// Annuler l'ordre uniquement si le statut est "buy" ou "sell"
	if status == "buy" || status == "sell" {
		var orderIdToCancel string
		if status == "buy" {
			orderIdToCancel = cycle.BuyId
			color.Yellow("Annulation de l'ordre d'achat %s", orderIdToCancel)
		} else {
			orderIdToCancel = cycle.SellId
			color.Yellow("Annulation de l'ordre de vente %s", orderIdToCancel)
		}

		// Nettoyer l'ID de l'ordre avec l'exchange spécifique
		cleanOrderId := cleanOrderId(orderIdToCancel, cycle.Exchange)
		if cleanOrderId == "" {
			color.Red("ID d'ordre invalide: %s", orderIdToCancel)
		} else {
			// Annuler l'ordre avec la fonction sécurisée
			success, err := safeOrderCancel(client, cleanOrderId, cycle.IdInt)

			if !success && err != nil {
				color.Red("Échec de l'annulation de l'ordre: %v", err)
				// Demander confirmation pour continuer malgré l'erreur
				color.Yellow("Voulez-vous quand même supprimer le cycle de la base de données? (o/n): ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "o" && strings.ToLower(response) != "oui" {
					color.Red("Annulation abandonnée.")
					os.Exit(1)
				}
			} else {
				color.Green("Ordre annulé avec succès!")
			}
		}
	} else {
		color.Yellow("Le cycle a le statut '%s', aucun ordre à annuler, suppression de la base de données uniquement", status)
	}

	// Supprimer le cycle de la base de données, même si l'annulation de l'ordre a échoué
	// mais que l'utilisateur a confirmé la suppression
	err = repo.DeleteByIdInt(int32(idInt))
	if err != nil {
		color.Red("Erreur lors de la suppression du cycle: %v", err)
		os.Exit(1)
	}
	color.Green("Cycle %d supprimé avec succès", idInt)
}
