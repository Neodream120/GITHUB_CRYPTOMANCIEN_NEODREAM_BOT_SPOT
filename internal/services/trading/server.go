package commands

import (
	"fmt"
	"html/template"
	"log"
	"main/internal/config"
	"main/internal/database"
	"net/http"
	"strings"
	"time"
)

// Template HTML intégré directement dans le code - version améliorée avec accumulation
const htmlTemplate = `<!DOCTYPE html>
<html lang="fr">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Cryptomancien - Neodream Bot - Tableau de bord</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.3/dist/css/bootstrap.min.css">
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/flatpickr/dist/flatpickr.min.css">
    <script src="https://cdn.jsdelivr.net/npm/flatpickr"></script>
    <script src="https://cdn.jsdelivr.net/npm/flatpickr/dist/l10n/fr.js"></script>
    
    <style>
        body {
            padding-top: 20px;
            background-color: #f8f9fa;
        }
        .status-buy {
            color: #28a745;
            font-weight: bold;
        }
        .status-sell {
            color: #ffc107;
            font-weight: bold;
        }
        .status-completed {
            color: #0275d8;
            font-weight: bold;
        }
        .status-cancelled {
            color: #d9534f;
            font-weight: bold;
        }
        .profit-positive {
            color: #28a745;
        }
        .profit-negative {
            color: #d9534f;
        }
        .header-buttons {
            margin-bottom: 20px;
        }
        .filter-card {
            background-color: #fff;
            border-radius: 0.5rem;
            box-shadow: 0 0.125rem 0.25rem rgba(0, 0, 0, 0.075);
            margin-bottom: 1.5rem;
            padding: 1rem;
        }
        .nav-pills .nav-link {
            margin-right: 0.5rem;
        }
        .tax-important {
            background-color: #fff3cd;
            padding: 0.5rem;
            border-radius: 0.25rem;
            font-weight: bold;
        }
        .tax-badge {
            padding: 0.35em 0.65em;
            font-size: 0.75em;
            font-weight: 700;
            border-radius: 0.25rem;
            margin-left: 0.5rem;
        }
		.exchange-order-id {
			word-wrap: break-word;  /* Permettre le retour à la ligne */
			font-size: 0.4em;  /* Réduire la taille de police */
			overflow: hidden;  /* Cacher le contenu qui dépasse */
			text-overflow: ellipsis;  /* Ajouter des points de suspension (...) si trop long */
			white-space: normal;  /* Autoriser le retour à la ligne */
		}	
    </style>
</head>
<body>
<input type="hidden" id="accumulationField" name="accumulation" value="{{ if .showAccumulation }}true{{ else }}false{{ end }}">
    <div class="container">
        <h1 class="mb-4">Cryptomancien - Neodream - Bot - Tableau de bord</h1>
        
        <!-- Filtres améliorés -->
        <div class="filter-card">
            <form id="filtersForm" method="get" action="/">
                <div class="row g-3 align-items-end">
                    <!-- Vue -->
                    <div class="col-md-3">
                        <label class="form-label">Vue</label>
                        <div class="btn-group w-100" role="group">
                            <input type="radio" class="btn-check" name="complete" id="allCycles" value="false" autocomplete="off" {{ if not .showCompleted }}checked{{ end }}>
                            <label class="btn btn-outline-primary" for="allCycles">Tous les cycles</label>
                            
                            <input type="radio" class="btn-check" name="complete" id="completedCycles" value="true" autocomplete="off" {{ if .showCompleted }}checked{{ end }}>
                            <label class="btn btn-outline-primary" for="completedCycles">Complétés</label>
                        </div>
                    </div>
                    
                    <!-- Exchange -->
                    <div class="col-md-3">
                        <label for="exchangeFilter" class="form-label">Exchange</label>
                        <select id="exchangeFilter" name="exchange" class="form-select">
                            <option value="">Tous les exchanges</option>
                            {{ range .exchanges }}
                                <option value="{{ . }}" {{ if eq $.exchangeFilter . }}selected{{ end }}>{{ . }}</option>
                            {{ end }}
                        </select>
                    </div>
                    
                    <!-- Période -->
                    <div class="col-md-3">
                        <label for="periodFilter" class="form-label">Période</label>
                        <select id="periodFilter" name="period" class="form-select">
                            <option value="">Toutes les périodes</option>
                            {{ range .periodOptions }}
                                <option value="{{ .value }}" {{ if eq $.periodFilter .value }}selected{{ end }}>{{ .label }}</option>
                            {{ end }}
                        </select>
                    </div>
                    
                    <div class="col-md-3">
                        <label class="form-label">Vue spéciale</label>
                        <select id="viewMode" name="view_mode" class="form-select" onchange="toggleViewMode(this.value)">
                            <option value="cycles" {{ if not .showAccumulation }}selected{{ end }}>Cycles de trading</option>
                            <option value="accumulation" {{ if .showAccumulation }}selected{{ end }}>Accumulations</option>
                        </select>
                    </div>
                </div>
                
                <!-- Dates personnalisées - affichées uniquement si aucune période n'est sélectionnée -->
                <div class="row g-3 mt-2" id="customDatesRow">
                    <div class="col-md-4">
                        <label for="startDate" class="form-label">Date de début</label>
                        <input type="date" id="startDate" name="start_date" class="form-control" value="{{ .startDate }}">
                    </div>
                    <div class="col-md-4">
                        <label for="endDate" class="form-label">Date de fin</label>
                        <input type="date" id="endDate" name="end_date" class="form-control" value="{{ .endDate }}">
                    </div>
                    <div class="col-md-4 d-flex align-items-end">
                        <button type="submit" class="btn btn-primary me-2">Filtrer</button>
                        <a href="/" class="btn btn-outline-secondary">Réinitialiser</a>
                    </div>
                </div>
            </form>
        </div>

        <!-- Statistiques générales -->
        <div class="row mb-4">
            <div class="col-md-3">
                <div class="card bg-light">
                    <div class="card-body">
                        <h5 class="card-title">Cycles totaux</h5>
                        <p class="card-text fs-4">{{ .cyclesCount }}</p>
                    </div>
                </div>
            </div>
            <div class="col-md-3">
                <div class="card bg-success text-white">
                    <div class="card-body">
                        <h5 class="card-title">Cycles d'achat</h5>
                        <p class="card-text fs-4">{{ .buyCycles }}</p>
                    </div>
                </div>
            </div>
            <div class="col-md-3">
                <div class="card bg-warning">
                    <div class="card-body">
                        <h5 class="card-title">Cycles de vente</h5>
                        <p class="card-text fs-4">{{ .sellCycles }}</p>
                    </div>
                </div>
            </div>
            <div class="col-md-3">
                <div class="card bg-primary text-white">
                    <div class="card-body">
                        <h5 class="card-title">Cycles complétés</h5>
                        <p class="card-text fs-4">{{ .cyclesCompleted }}</p>
                    </div>
                </div>
            </div>
        </div>

        <div class="row mb-4">
            <div class="col-md-4">
                <div class="card bg-light">
                    <div class="card-body">
                        <h5 class="card-title">Volume total d'achat</h5>
                        <p class="card-text fs-4">{{ printf "%.2f" .totalBuy }} USDC</p>
                    </div>
                </div>
            </div>
            <div class="col-md-4">
                <div class="card bg-light">
                    <div class="card-body">
                        <h5 class="card-title">Volume total de vente</h5>
                        <p class="card-text fs-4">{{ printf "%.2f" .totalSell }} USDC</p>
                    </div>
                </div>
            </div>
            <div class="col-md-4">
                <div class="card {{ if gt .gainAbs 0.0 }}bg-success text-white{{ else }}bg-danger text-white{{ end }}">
                    <div class="card-body">
                        <h5 class="card-title">Gain total</h5>
                        <p class="card-text fs-4">
                            {{ printf "%.2f" .gainAbs }} USDC ({{ printf "%.2f" .gainPercent }}%)
                        </p>
                    </div>
                </div>
            </div>
        </div>
		

        {{ if .showAccumulation }}
        <!-- Début de la section à remplacer pour les cycles (pas les accumulations) -->

        <h2 class="mb-3">
            {{ if .showCompleted }}
                Cycles complétés
            {{ else }}
                {{ if .showAll }}Tous les cycles{{ else }}Cycles actifs{{ end }}
            {{ end }}
            {{ if .exchangeFilter }} - {{ .exchangeFilter }}{{ end }}
            {{ if .periodFilter }} - {{ .periodFilter }}{{ end }}
            {{ if .startDate }} - Du {{ .startDate }}{{ end }}
            {{ if .endDate }} au {{ .endDate }}{{ end }}
        </h2>

        <div class="table-responsive">
            <table class="table table-striped">
                <thead>
					<tr>
						<th>ID</th>
						<th>Exchange</th>
						<th>Statut</th>
						<th>Date achat</th>
						<th>Date vente</th>
						<th>Quantité BTC</th>
						<th>Montant USDC</th>
						<th>Montant vente</th>
						<th>Gains</th>
						<!-- Suppression de la colonne "Frais" -->
						<th>Année fiscale</th>
						<th>Durée</th>
						<th>ID Exchange Ordre Achat</th>
						<th>ID Exchange Ordre Vente</th>
					</tr>
				</thead>
				<tbody>
					{{ range .Cycles }}
					<tr>
						<td>{{ .idInt }}</td>
						<td>{{ .exchange }}</td>
						<td class="status-{{ .status }}">{{ .formattedStatus }}</td>
						<td>{{ .buyDate }}</td>
						<td>{{ .sellDateFormatted }}</td>
						<td>{{ printf "%.8f" .quantity }}</td>
						<td>{{ printf "%.8f" .buyTotal }}</td>
						<td>
							{{ if eq .status "completed" }}{{ printf "%.8f" .sellTotal }}
							{{ else if eq .status "sell" }}{{ printf "%.8f" .sellTotal }}
							{{ else }}-{{ end }}
						</td>
						<td class="{{ if gt .profit 0.0 }}profit-positive{{ else if lt .profit 0.0 }}profit-negative{{ end }}">
							{{ if eq .status "completed" }}
								{{ printf "%.8f" .profit }} ({{ printf "%.2f" .profitPercentage }}%)
							{{ else if eq .status "sell" }}
								{{ printf "%.8f" .profit }} ({{ printf "%.2f" .profitPercentage }}%)
							{{ else }}
								-
							{{ end }}
						</td>
						<!-- Suppression de l'affichage des frais -->
						<td>
							{{ .taxYear }}
							{{ if eq .status "completed" }}
								{{ if .declareThisYear }}
								<span class="badge bg-danger tax-badge">À déclarer</span>
								{{ end }}
							{{ end }}
						</td>
						<td>{{ if .formattedDuration }}{{ .formattedDuration }}{{ else }}{{ formatAge .age }}{{ end }}</td>
						<td><small class="exchange-order-id">{{ .buyId }}</small></td>
						<td><small class="exchange-order-id">{{ .sellId }}</small></td>
					</tr>
					{{ end }}
				</tbody>
            </table>
        </div>

        {{ if .hasAccumulations }}
        <div class="row mb-4">
            <div class="col-12">
                <h3 class="mb-3">Détail des accumulations</h3>
                <div class="table-responsive">
                    <table class="table table-striped small">
                        <thead>
							<tr>
								<th>ID</th>
								<th>Exchange</th>
								<th>Statut</th>
								<th>Date achat</th>
								<th>Date vente</th>
								<th>Quantité BTC</th>
								<th>Montant USDC</th>
								<th>Montant vente</th>
								<th>Gains</th>
								<!-- Suppression de la colonne "Frais" -->
								<th>Année fiscale</th>
								<th>Durée</th>
								<th>ID Exchange Ordre Achat</th>
								<th>ID Exchange Ordre Vente</th>
							</tr>
						</thead>
						<tbody>
							{{ range .Cycles }}
							<tr>
								<td>{{ .idInt }}</td>
								<td>{{ .exchange }}</td>
								<td class="status-{{ .status }}">{{ .formattedStatus }}</td>
								<td>{{ .buyDate }}</td>
								<td>{{ .sellDateFormatted }}</td>
								<td>{{ printf "%.8f" .quantity }}</td>
								<td>{{ printf "%.8f" .buyTotal }}</td>
								<td>
									{{ if eq .status "completed" }}{{ printf "%.8f" .sellTotal }}
									{{ else if eq .status "sell" }}{{ printf "%.8f" .sellTotal }}
									{{ else }}-{{ end }}
								</td>
								<td class="{{ if gt .profit 0.0 }}profit-positive{{ else if lt .profit 0.0 }}profit-negative{{ end }}">
									{{ if eq .status "completed" }}
										{{ printf "%.8f" .profit }} ({{ printf "%.2f" .profitPercentage }}%)
									{{ else if eq .status "sell" }}
										{{ printf "%.8f" .profit }} ({{ printf "%.2f" .profitPercentage }}%)
									{{ else }}
										-
									{{ end }}
								</td>
								<!-- Suppression de l'affichage des frais -->
								<td>
									{{ .taxYear }}
									{{ if eq .status "completed" }}
										{{ if .declareThisYear }}
										<span class="badge bg-danger tax-badge">À déclarer</span>
										{{ end }}
									{{ end }}
								</td>
								<td>{{ if .formattedDuration }}{{ .formattedDuration }}{{ else }}{{ formatAge .age }}{{ end }}</td>
								<td><small class="exchange-order-id">{{ .buyId }}</small></td>
								<td><small class="exchange-order-id">{{ .sellId }}</small></td>
							</tr>
							{{ end }}
						</tbody>
                    </table>
                </div>
            </div>
        </div>
        {{ end }}
        {{ else }}
        <h2 class="mb-3">
            {{ if .showCompleted }}
                Cycles complétés
            {{ else }}
                {{ if .showAll }}Tous les cycles{{ else }}Cycles actifs{{ end }}
            {{ end }}
            {{ if .exchangeFilter }} - {{ .exchangeFilter }}{{ end }}
            {{ if .periodFilter }} - {{ .periodFilter }}{{ end }}
            {{ if .startDate }} - Du {{ .startDate }}{{ end }}
            {{ if .endDate }} au {{ .endDate }}{{ end }}
        </h2>

        <div class="table-responsive">
            <table class="table table-striped">
                							<tr>
								<th>ID</th>
								<th>Exchange</th>
								<th>Statut</th>
								<th>Date achat</th>
								<th>Date vente</th>
								<th>Quantité BTC</th>
								<th>Montant USDC</th>
								<th>Montant vente</th>
								<th>Gains</th>
								<!-- Suppression de la colonne "Frais" -->
								<th>Année fiscale</th>
								<th>Durée</th>
								<th>ID Exchange Ordre Achat</th>
								<th>ID Exchange Ordre Vente</th>
							</tr>
						</thead>
						<tbody>
							{{ range .Cycles }}
							<tr>
								<td>{{ .idInt }}</td>
								<td>{{ .exchange }}</td>
								<td class="status-{{ .status }}">{{ .formattedStatus }}</td>
								<td>{{ .buyDate }}</td>
								<td>{{ .sellDateFormatted }}</td>
								<td>{{ printf "%.8f" .quantity }}</td>
								<td>{{ printf "%.8f" .buyTotal }}</td>
								<td>
									{{ if eq .status "completed" }}{{ printf "%.8f" .sellTotal }}
									{{ else if eq .status "sell" }}{{ printf "%.8f" .sellTotal }}
									{{ else }}-{{ end }}
								</td>
								<td class="{{ if gt .profit 0.0 }}profit-positive{{ else if lt .profit 0.0 }}profit-negative{{ end }}">
									{{ if eq .status "completed" }}
										{{ printf "%.8f" .profit }} ({{ printf "%.2f" .profitPercentage }}%)
									{{ else if eq .status "sell" }}
										{{ printf "%.8f" .profit }} ({{ printf "%.2f" .profitPercentage }}%)
									{{ else }}
										-
									{{ end }}
								</td>
								<!-- Suppression de l'affichage des frais -->
								<td>
									{{ .taxYear }}
									{{ if eq .status "completed" }}
										{{ if .declareThisYear }}
										<span class="badge bg-danger tax-badge">À déclarer</span>
										{{ end }}
									{{ end }}
								</td>
								<td>{{ if .formattedDuration }}{{ .formattedDuration }}{{ else }}{{ formatAge .age }}{{ end }}</td>
								<td><small class="exchange-order-id">{{ .buyId }}</small></td>
								<td><small class="exchange-order-id">{{ .sellId }}</small></td>
							</tr>
							{{ end }}
						</tbody>
            </table>
        </div>

        <!-- Récapitulatif fiscal -->
        <div class="row mt-5 mb-4">
            <div class="col-12">
                <h3>Récapitulatif fiscal</h3>
                <div class="alert alert-warning">
                    <p><strong>Note importante:</strong> Ce récapitulatif est fourni à titre indicatif et ne constitue pas un document fiscal officiel.</p>
                    <p>Pour la déclaration des plus-values sur actifs numériques (formulaire 2086), merci de consulter un expert-comptable.</p>
                </div>
                
                <div class="card mb-4">
                    <div class="card-header">
                        <h5>Profits par année fiscale</h5>
                    </div>
                    <div class="card-body">
                        <table class="table">
                            <thead>
                                <tr>
                                    <th>Année</th>
                                    <th>Profits totaux (USDC)</th>
                                    <th>Impôt estimé (30%)</th>
                                    <th>Statut</th>
                                </tr>
                            </thead>
                            <tbody>
                                {{ range $year, $profit := .taxYearProfits }}
                                <tr {{ if eq $year $.currentTaxYear }}class="tax-important"{{ end }}>
                                    <td><strong>{{ $year }}</strong></td>
                                    <td class="{{ if gt $profit 0.0 }}profit-positive{{ else if lt $profit 0.0 }}profit-negative{{ end }}">
                                        {{ printf "%.2f" $profit }}
                                    </td>
                                    <td>{{ printf "%.2f" (mul $profit 0.3) }}</td>
                                    <td>
                                        {{ if eq $year $.currentTaxYear }}
                                            <span class="badge bg-danger">À déclarer en {{ add $year 1 }}</span>
                                        {{ else if lt $year $.currentTaxYear }}
                                            <span class="badge bg-success">Déclaration passée</span>
                                        {{ else }}
                                            <span class="badge bg-info">Année future</span>
                                        {{ end }}
                                    </td>
                                </tr>
                                {{ end }}
                                <tr class="table-secondary">
                                    <td colspan="2"><strong>Total estimé des impôts à payer</strong></td>
                                    <td><strong>{{ printf "%.2f" .totalTaxEstimate }}</strong></td>
                                    <td></td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                    <div class="card-footer text-muted">
                        <p><strong>Rappel</strong> : En France, les plus-values sur actifs numériques sont soumises à un taux forfaitaire de 30% (12,8% d'impôt sur le revenu + 17,2% de prélèvements sociaux) au-delà d'un seuil de cession annuel de 305€.</p>
                        <p>Le total des frais liés aux transactions peut être déduit du montant imposable. Conservez tous les justificatifs de frais.</p>
                    </div>
                </div>
                
                <div class="card mb-4">
                    <div class="card-header">
                        <h5>Documents à conserver pour le FISC</h5>
                    </div>
                    <div class="card-body">
                        <p>Pour justifier vos opérations sur actifs numériques, conservez les éléments suivants pour chaque transaction :</p>
                        <ul>
                            <li><strong>Date et heure</strong> de chaque transaction (achat et vente)</li>
                            <li><strong>Identifiants de transaction</strong> (ID des ordres)</li>
                            <li><strong>Nature de l'opération</strong> (achat, vente, échange)</li>
                            <li><strong>Contreparties utilisées</strong> (crypto/fiat)</li>
                            <li><strong>Frais de transaction</strong> payés</li>
                            <li><strong>Relevés de compte</strong> des plateformes d'échange</li>
                        </ul>
                        <p>Il est recommandé de conserver ces documents pendant au moins 6 ans, durée pendant laquelle l'administration fiscale peut exercer son droit de contrôle.</p>
                    </div>
					<div class="card-footer text-muted">
						<p><strong>Note</strong> : Les gains fiscaux affichés incluent une déduction supplémentaire de 0.2% pour frais de transaction. Comme les prix d'achat et de vente incluent déjà les frais d'exchange, cette déduction peut être optionnelle selon votre situation.</p>
					</div>
                </div>
            </div>
        </div>
        {{ end }}

        <div class="mt-4 text-muted">
            <p>Dernière mise à jour: {{ .currentTime }}</p>
        </div>
    </div>

    <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.3/dist/js/bootstrap.bundle.min.js"></script>
    <script>
        // Gestion du champ période et dates personnalisées
        document.addEventListener('DOMContentLoaded', function() {
            const periodFilter = document.getElementById('periodFilter');
            const customDatesRow = document.getElementById('customDatesRow');
            const startDateInput = document.getElementById('startDate');
            const endDateInput = document.getElementById('endDate');
            
            // Fonction pour gérer l'affichage des dates personnalisées
            function toggleCustomDates() {
                if (periodFilter.value === '') {
                    customDatesRow.style.display = 'flex';
                } else {
                    // Effacer les dates si une période est sélectionnée
                    startDateInput.value = '';
                    endDateInput.value = '';
                    customDatesRow.style.display = 'flex';
                }
            }
            
            // Initialiser l'état
            toggleCustomDates();
            
            // Écouter les changements
            periodFilter.addEventListener('change', toggleCustomDates);
            
            // Soumission du formulaire
            document.getElementById('filtersForm').addEventListener('submit', function(e) {
                // Si une période est sélectionnée, supprimer les dates de la requête
                if (periodFilter.value !== '') {
                    startDateInput.disabled = true;
                    endDateInput.disabled = true;
                }
            });
        });

        // Fonction pour basculer entre les modes de vue
        function toggleViewMode(mode) {
            const accumulationField = document.getElementById('accumulationField');
            
            if (mode === 'accumulation') {
                accumulationField.value = 'true';
            } else {
                accumulationField.value = 'false';
            }
            
            // Soumettre le formulaire automatiquement pour changer de vue
            document.getElementById('filtersForm').submit();
        }
    </script>
</body>
</html>
`

// Server démarre un serveur HTTP pour afficher et gérer les cycles
func Server() {
	fmt.Println("Démarrage du serveur sur http://localhost:8080")
	fmt.Println("Appuyez sur Ctrl+C pour arrêter le serveur")

	// Initialiser le router
	mux := http.NewServeMux()

	// Route principale pour afficher les cycles avec tous les filtres possibles
	mux.HandleFunc("/", handleDashboard)

	// Route pour mettre à jour les cycles
	mux.HandleFunc("/update", handleUpdate)

	// Démarrer le serveur
	err := http.ListenAndServe("localhost:8080", mux)
	if err != nil {
		log.Fatal(err)
	}
}

// formatStatus retourne un statut formaté pour l'affichage
func formatStatus(c *database.Cycle) string {
	switch c.Status {
	case "buy":
		return "Achat en cours"
	case "sell":
		return "Vente en cours"
	case "completed":
		return "Complété"
	case "cancelled":
		return "Annulé"
	default:
		return c.Status
	}
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Récupérer les paramètres de filtrage
	queryParams := r.URL.Query()

	// 1. Filtrage par status de complétion
	showCompletedOnly := queryParams.Get("complete") == "true"

	// 2. Filtrage par exchange
	exchangeFilter := queryParams.Get("exchange")

	// 3. Filtrage par période prédéfinie
	periodFilter := queryParams.Get("period") // Valeurs possibles: 7j, 30j, 90j, 180j, 365j

	// 4. Filtrage par dates personnalisées
	startDateStr := queryParams.Get("start_date") // Format: YYYY-MM-DD
	endDateStr := queryParams.Get("end_date")     // Format: YYYY-MM-DD

	// 5. Afficher uniquement les accumulations
	showAccumulation := queryParams.Get("accumulation") == "true"

	// Calculer les dates de début et de fin en fonction des filtres
	startDate, endDate := calculateDateRange(periodFilter, startDateStr, endDateStr)

	// Récupérer le repository
	repo := database.GetRepository()

	// Récupérer la configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, "Erreur lors du chargement de la configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Récupérer tous les cycles
	allCycles, err := repo.FindAll()
	if err != nil {
		http.Error(w, "Erreur lors de la récupération des cycles: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Filtrer les cycles selon les critères
	var cycles []*database.Cycle
	for _, cycle := range allCycles {
		// Critère 1: Filtrage par complétion
		if showCompletedOnly && cycle.Status != "completed" {
			continue
		}

		// Critère 2: Filtrage par exchange
		if exchangeFilter != "" && !strings.EqualFold(cycle.Exchange, exchangeFilter) {
			continue
		}

		// Critère 3 & 4: Filtrage par date
		if !isCycleInDateRange(cycle, startDate, endDate) {
			continue
		}

		// Inclure ce cycle dans les résultats filtrés
		cycles = append(cycles, cycle)
	}

	// Convertir les cycles en DTOs pour l'affichage
	var cyclesDTO []map[string]interface{}
	for _, cycle := range cycles {
		// Créer le DTO de base
		dto := convertCycleToDTO(cycle)

		// Calcul précis des montants d'achat
		buyTotal := cycle.BuyPrice * cycle.Quantity

		// Initialiser les valeurs de vente et de profit à zéro par défaut
		sellTotal := 0.0
		grossProfit := 0.0
		grossProfitPercentage := 0.0

		// Calculer les montants de vente et profits uniquement pour les cycles complétés ou en vente
		if cycle.Status == "completed" || cycle.Status == "sell" {
			sellTotal = cycle.SellPrice * cycle.Quantity
			grossProfit = sellTotal - buyTotal

			// Calculer le pourcentage de profit seulement si buyTotal est supérieur à zéro
			if buyTotal > 0 {
				grossProfitPercentage = (grossProfit / buyTotal) * 100
			}
		}

		// Mettre à jour le DTO avec les valeurs calculées
		dto["buyTotal"] = buyTotal
		dto["sellTotal"] = sellTotal
		dto["profit"] = grossProfit
		dto["profitPercentage"] = grossProfitPercentage
		dto["originalBuyOrderId"] = cycle.BuyId   // L'ID original de l'ordre d'achat
		dto["originalSellOrderId"] = cycle.SellId // L'ID original de l'ordre de vente

		// Date d'achat formatée au format français
		dto["buyDate"] = cycle.CreatedAt.Format("02/01/2006 15:04")

		// Informations fiscales
		dto["taxYear"] = cycle.CreatedAt.Year()
		if cycle.Status == "completed" {
			sellDate := cycle.CompletedAt
			if !sellDate.IsZero() {
				dto["sellTaxYear"] = sellDate.Year()
				// Indiquer si le profit doit être déclaré cette année
				currentYear := time.Now().Year()
				dto["declareThisYear"] = (sellDate.Year() == currentYear)
			} else {
				dto["sellTaxYear"] = "-"
				dto["declareThisYear"] = false
			}
		} else {
			dto["sellTaxYear"] = "-"
			dto["declareThisYear"] = false
		}

		cyclesDTO = append(cyclesDTO, dto)
	}

	// Calculer les statistiques pour les cycles filtrés
	filteredStats := calculateFilteredCycleStatistics(cycles)

	// Calculer les profits par année fiscale
	taxYearProfits := calculateProfitsByTaxYear(cycles)

	// Préparer les données pour le template
	data := map[string]interface{}{
		"Cycles":           cyclesDTO,
		"cyclesCount":      len(cycles),
		"buyCycles":        filteredStats.buyCycles,
		"sellCycles":       filteredStats.sellCycles,
		"cyclesCompleted":  filteredStats.completedCycles,
		"totalBuy":         filteredStats.totalBuy,
		"totalSell":        filteredStats.totalSell,
		"gainAbs":          filteredStats.gainAbs,
		"gainPercent":      filteredStats.gainPercent,
		"currentTime":      time.Now().Format("02/01/2006 15:04:05"),
		"showAll":          !showCompletedOnly,
		"showCompleted":    showCompletedOnly,
		"showAccumulation": showAccumulation,
		"exchangeFilter":   exchangeFilter,
		"periodFilter":     periodFilter,
		"startDate":        startDateStr,
		"endDate":          endDateStr,
		"exchanges":        getAvailableExchanges(cfg),
		"periodOptions":    getPeriodOptions(),
		"currentTaxYear":   time.Now().Year(),
		"taxYearProfits":   taxYearProfits,
		"totalTaxEstimate": calculateTotalTaxEstimate(taxYearProfits),
	}

	// Si on affiche les accumulations, récupérer les données d'accumulation
	if showAccumulation {
		accuRepo := database.GetAccumulationRepository()

		// Récupérer toutes les accumulations
		allAccumulations, err := accuRepo.FindAll()
		if err != nil {
			http.Error(w, "Erreur lors de la récupération des accumulations: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Filtrer les accumulations selon les mêmes critères
		var filteredAccumulations []*database.Accumulation
		for _, accu := range allAccumulations {
			// Filtrage par exchange
			if exchangeFilter != "" && !strings.EqualFold(accu.Exchange, exchangeFilter) {
				continue
			}

			// Filtrage par date
			if !isAccumulationInDateRange(accu, startDate, endDate) {
				continue
			}

			filteredAccumulations = append(filteredAccumulations, accu)
		}

		// Convertir les accumulations en DTOs pour l'affichage
		var accumulationsDTO []map[string]interface{}
		for _, accu := range filteredAccumulations {
			dto := map[string]interface{}{
				"idInt":              accu.IdInt,
				"exchange":           accu.Exchange,
				"quantity":           accu.Quantity,
				"originalBuyPrice":   accu.OriginalBuyPrice,
				"targetSellPrice":    accu.TargetSellPrice,
				"cancelPrice":        accu.CancelPrice,
				"deviation":          accu.Deviation,
				"createdAtFormatted": accu.CreatedAt.Format("02/01/2006 15:04:05"),
				"taxYear":            accu.CreatedAt.Year(),
			}
			accumulationsDTO = append(accumulationsDTO, dto)
		}

		// Récupérer les statistiques d'accumulation par exchange
		accumulationStats := make(map[string]map[string]interface{})
		for exchangeName, exchangeConfig := range cfg.Exchanges {
			if exchangeConfig.Enabled {
				if exchangeFilter == "" || strings.EqualFold(exchangeName, exchangeFilter) {
					stats, err := accuRepo.GetExchangeAccumulationStats(exchangeName)
					if err != nil {
						continue
					}

					accumulationStats[exchangeName] = map[string]interface{}{
						"enabled":          exchangeConfig.Accumulation,
						"count":            stats["count"],
						"totalQuantity":    stats["totalQuantity"],
						"savedValue":       stats["savedValue"],
						"averageDeviation": stats["averageDeviation"],
					}
				}
			}
		}

		// Ajouter les données d'accumulation au template
		data["allAccumulations"] = accumulationsDTO
		data["accumulationStats"] = accumulationStats
		data["hasAccumulations"] = len(filteredAccumulations) > 0
	}

	// Créer un template avec des fonctions auxiliaires
	funcMap := template.FuncMap{

		"mul": func(a, b float64) float64 {
			return a * b
		},
		"add": func(a, b int) int {
			return a + b
		},
		"formatAge": func(durationInDays float64) string {
			// Convertir en heures pour faciliter les comparaisons
			hours := durationInDays * 24

			if hours < 24 {
				// Moins de 24 heures
				h := int(hours)
				m := int((hours - float64(h)) * 60)
				if h == 0 {
					// Si moins d'une heure, afficher uniquement les minutes
					return fmt.Sprintf("%dm", m)
				}
				return fmt.Sprintf("%dh %dm", h, m)
			} else if durationInDays < 7 {
				// Entre 1 et 7 jours
				days := int(durationInDays)
				remainingHours := int(hours) % 24
				return fmt.Sprintf("%dj %dh", days, remainingHours)
			} else if durationInDays < 35 {
				// Entre 7 et 35 jours (5 semaines)
				weeks := int(durationInDays / 7)
				remainingDays := int(durationInDays) % 7
				return fmt.Sprintf("%dsem %dj", weeks, remainingDays)
			} else {
				// Plus de 5 semaines
				months := int(durationInDays / 30)
				remainingDays := int(durationInDays) % 30
				return fmt.Sprintf("%dmois %dj", months, remainingDays)
			}
		},
	}

	// Utiliser le funcMap lors de la création du template
	tmpl, err := template.New("index").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		http.Error(w, "Erreur lors de la compilation du template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Exécuter le template
	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Erreur lors du rendu du template: "+err.Error(), http.StatusInternalServerError)
	}
}

// Calcule les profits par année fiscale (utile pour les déclarations d'impôts)
func calculateProfitsByTaxYear(cycles []*database.Cycle) map[int]float64 {
	profitsByYear := make(map[int]float64)

	for _, cycle := range cycles {
		if cycle.Status == "completed" {
			// Pour simplifier, nous considérons que la date fiscale est la date de création
			// Dans un système idéal, vous utiliseriez la date de vente effective
			year := cycle.CreatedAt.Year()

			// Calcul des montants et frais
			buyTotal := cycle.BuyPrice * cycle.Quantity
			sellTotal := cycle.SellPrice * cycle.Quantity

			// Calcul des frais (0.1% pour l'achat et 0.1% pour la vente)
			//buyFees := buyTotal * 0.001
			//sellFees := sellTotal * 0.001
			//totalFees := buyFees + sellFees

			// Calcul du profit net (après déduction des frais)
			grossProfit := sellTotal - buyTotal
			netProfit := grossProfit

			// Ajouter le profit net à l'année fiscale correspondante
			profitsByYear[year] += netProfit
		}
	}

	return profitsByYear
}

// Calcule l'estimation des impôts totaux à payer (30% en France)
func calculateTotalTaxEstimate(profitsByYear map[int]float64) float64 {
	var totalTax float64

	// Calculer l'impôt pour chaque année
	for _, profit := range profitsByYear {
		if profit > 0 {
			totalTax += profit * 0.30
		}
	}

	return totalTax
}

// Structure complète pour les statistiques filtrées
type filteredStatsData struct {
	totalBuy        float64
	totalSell       float64
	gainAbs         float64
	gainPercent     float64
	buyCycles       int
	sellCycles      int
	completedCycles int
}

// Gestionnaire pour la mise à jour des cycles
func handleUpdate(w http.ResponseWriter, r *http.Request) {
	// Appeler la commande Update() pour mettre à jour les cycles
	Update()

	// Rediriger vers la page principale avec les mêmes paramètres de filtre
	http.Redirect(w, r, "/"+r.URL.RawQuery, http.StatusSeeOther)
}

// Calcule les statistiques complètes pour un ensemble de cycles filtrés
func calculateFilteredCycleStatistics(cycles []*database.Cycle) filteredStatsData {
	var stats filteredStatsData

	// Initialiser les compteurs
	stats.buyCycles = 0
	stats.sellCycles = 0
	stats.completedCycles = 0

	// Créer des maps pour vérifier les totaux par exchange
	exchangeTotals := make(map[string]struct {
		buy, sell float64
		completed int
	})

	// Calculer les totaux et les compteurs
	for _, cycle := range cycles {
		// Mettre à jour les statistiques par exchange
		exchangeStats := exchangeTotals[cycle.Exchange]

		switch cycle.Status {
		case "buy":
			stats.buyCycles++
		case "sell":
			stats.sellCycles++
		case "completed":
			stats.completedCycles++
			buyValue := cycle.BuyPrice * cycle.Quantity
			sellValue := cycle.SellPrice * cycle.Quantity

			stats.totalBuy += buyValue
			stats.totalSell += sellValue

			// Mise à jour des stats par exchange
			exchangeStats.buy += buyValue
			exchangeStats.sell += sellValue
			exchangeStats.completed++
		}

		exchangeTotals[cycle.Exchange] = exchangeStats
	}

	// Log des totaux par exchange pour vérification
	for exchange, totals := range exchangeTotals {
		if totals.completed > 0 {
			profit := totals.sell - totals.buy
			profitPercent := 0.0
			if totals.buy > 0 {
				profitPercent = (profit / totals.buy) * 100
			}
			log.Printf("Exchange %s: %d cycles complétés, Total achat: %.2f, Total vente: %.2f, Profit: %.2f (%.2f%%)",
				exchange, totals.completed, totals.buy, totals.sell, profit, profitPercent)
		}
	}

	// Calculer les gains
	stats.gainAbs = stats.totalSell - stats.totalBuy
	if stats.totalBuy > 0 {
		stats.gainPercent = (stats.gainAbs / stats.totalBuy) * 100
	}

	return stats
}

// Calcule la plage de dates en fonction des filtres
func calculateDateRange(periodFilter, startDateStr, endDateStr string) (*time.Time, *time.Time) {
	var startDate, endDate *time.Time
	now := time.Now()

	// Si une période prédéfinie est spécifiée
	if periodFilter != "" {
		// Initialiser la date de fin à aujourd'hui
		end := now
		endDate = &end

		// Calculer la date de début selon la période
		var start time.Time
		switch periodFilter {
		case "7j":
			start = now.AddDate(0, 0, -7)
		case "30j":
			start = now.AddDate(0, 0, -30)
		case "90j":
			start = now.AddDate(0, 0, -90)
		case "180j":
			start = now.AddDate(0, 0, -180)
		case "365j":
			start = now.AddDate(0, 0, -365)
		default:
			// Période non reconnue, ne pas appliquer de filtre
			return nil, nil
		}
		startDate = &start
	} else {
		// Utiliser les dates personnalisées si spécifiées
		if startDateStr != "" {
			if parsedDate, err := time.Parse("2006-01-02", startDateStr); err == nil {
				startDate = &parsedDate
			}
		}

		if endDateStr != "" {
			if parsedDate, err := time.Parse("2006-01-02", endDateStr); err == nil {
				// Ajuster à la fin de la journée (23:59:59)
				parsedDate = parsedDate.Add(24*time.Hour - 1*time.Second)
				endDate = &parsedDate
			}
		}
	}

	return startDate, endDate
}

// Vérifie si un cycle est dans la plage de dates spécifiée
func isCycleInDateRange(cycle *database.Cycle, startDate, endDate *time.Time) bool {
	// Si aucune date n'est spécifiée, inclure tous les cycles
	if startDate == nil && endDate == nil {
		return true
	}

	// Vérifier la date de début si spécifiée
	if startDate != nil && cycle.CreatedAt.Before(*startDate) {
		return false
	}

	// Vérifier la date de fin si spécifiée
	if endDate != nil && cycle.CreatedAt.After(*endDate) {
		return false
	}

	return true
}

// Vérifie si une accumulation est dans la plage de dates spécifiée
func isAccumulationInDateRange(accu *database.Accumulation, startDate, endDate *time.Time) bool {
	// Si aucune date n'est spécifiée, inclure toutes les accumulations
	if startDate == nil && endDate == nil {
		return true
	}

	// Vérifier la date de début si spécifiée
	if startDate != nil && accu.CreatedAt.Before(*startDate) {
		return false
	}

	// Vérifier la date de fin si spécifiée
	if endDate != nil && accu.CreatedAt.After(*endDate) {
		return false
	}

	return true
}

// Récupère la liste des exchanges disponibles
func getAvailableExchanges(cfg *config.Config) []string {
	exchanges := []string{}

	// Ajouter les exchanges configurés et activés
	for name, exchange := range cfg.Exchanges {
		if exchange.Enabled {
			exchanges = append(exchanges, name)
		}
	}

	return exchanges
}

// Récupère les options de période disponibles
func getPeriodOptions() []map[string]string {
	return []map[string]string{
		{"value": "7j", "label": "7 derniers jours"},
		{"value": "30j", "label": "30 derniers jours"},
		{"value": "90j", "label": "3 derniers mois"},
		{"value": "180j", "label": "6 derniers mois"},
		{"value": "365j", "label": "Dernière année"},
	}
}

func formatDetailedDuration(ageInDays float64) string {
	// Convertir en heures pour faciliter les calculs
	hours := ageInDays * 24

	var formattedDuration string
	if hours < 24 {
		// Moins de 24 heures
		h := int(hours)
		m := int((hours - float64(h)) * 60)
		if h == 0 {
			formattedDuration = fmt.Sprintf("%dm", m)
		} else {
			formattedDuration = fmt.Sprintf("%dh %dm", h, m)
		}
	} else if ageInDays < 7 {
		// Entre 1 et 7 jours
		days := int(ageInDays)
		remainingHours := int(hours) % 24
		formattedDuration = fmt.Sprintf("%dj %dh", days, remainingHours)
	} else if ageInDays < 35 {
		// Entre 7 et 35 jours
		weeks := int(ageInDays / 7)
		remainingDays := int(ageInDays) % 7
		formattedDuration = fmt.Sprintf("%dsem %dj", weeks, remainingDays)
	} else {
		// Plus de 35 jours
		months := int(ageInDays / 30)
		remainingDays := int(ageInDays) % 30
		formattedDuration = fmt.Sprintf("%dmois %dj", months, remainingDays)
	}

	return formattedDuration
}

func convertCycleToDTO(cycle *database.Cycle) map[string]interface{} {
	dto := map[string]interface{}{
		"idInt":     cycle.IdInt,
		"exchange":  cycle.Exchange,
		"status":    cycle.Status,
		"quantity":  cycle.Quantity,
		"buyPrice":  cycle.BuyPrice,
		"buyId":     cycle.BuyId,
		"sellPrice": cycle.SellPrice,
		"sellId":    cycle.SellId,
		"age":       cycle.GetAge(),
		"taxYear":   cycle.CreatedAt.Year(),
	}

	// Informations standard
	dto["formattedStatus"] = formatStatus(cycle)
	dto["quantity"] = cycle.Quantity // Ajouter la quantité de BTC

	// Date d'achat formatée au format français
	dto["buyDate"] = cycle.CreatedAt.Format("02/01/2006 15:04")

	// Gestion des dates et informations fiscales
	switch cycle.Status {
	case "completed":
		if !cycle.CompletedAt.IsZero() {
			// Utiliser CompletedAt pour les années fiscales
			dto["sellTaxYear"] = cycle.CompletedAt.Year()

			// Vérifier si le profit doit être déclaré cette année
			currentYear := time.Now().Year()
			dto["declareThisYear"] = (cycle.CompletedAt.Year() == currentYear)
		} else {
			// Si CompletedAt est zéro, utiliser une estimation
			estimatedSellDate := estimateCompletionTime(cycle)
			dto["sellTaxYear"] = estimatedSellDate.Year()

			// Vérifier si l'année estimée correspond à l'année actuelle
			currentYear := time.Now().Year()
			dto["declareThisYear"] = (estimatedSellDate.Year() == currentYear)
		}
	default:
		// Pour les autres statuts
		dto["sellTaxYear"] = "-"
		dto["declareThisYear"] = false
	}

	switch cycle.Status {
	case "completed":
		if !cycle.CompletedAt.IsZero() {
			// Forcer le formatage explicite en français
			formattedSellDate := cycle.CompletedAt.Format("02/01/2006 15:04")
			// NOUVEAU : Vérification et correction potentielle
			if formattedSellDate != cycle.CompletedAt.Format("02/01/2006 15:04") {
				log.Printf("ALERTE: Incohérence dans le formatage de la date")
			}

			dto["sellDateFormatted"] = formattedSellDate

			// Calculer la durée
			cycleDuration := cycle.CompletedAt.Sub(cycle.CreatedAt)
			durationDays := cycleDuration.Hours() / 24

			dto["formattedDuration"] = formatDetailedDuration(durationDays)
		}
	}

	return dto
}

// Fonction pour estimer la date de complétion si elle est manquante
func estimateCompletionTime(cycle *database.Cycle) time.Time {
	// Estimer la date de complétion en fonction de l'exchange
	var estimatedDuration time.Duration
	switch cycle.Exchange {
	case "KUCOIN":
		estimatedDuration = 3 * time.Hour
	case "MEXC":
		estimatedDuration = 3 * time.Hour
	case "BINANCE":
		estimatedDuration = 3 * time.Hour
	case "KRAKEN": // Assurez-vous que ce cas existe
		estimatedDuration = 3 * time.Hour
	default:
		estimatedDuration = 3 * time.Hour
	}

	return cycle.CreatedAt.Add(estimatedDuration)
}
