package commands

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"main/internal/config"
	"main/internal/database"
	"net/http"
	"sort"
	"time"
)

// StatsServer démarre un serveur HTTP dédié aux statistiques avancées
func StatsServer() {
	fmt.Println("Démarrage du serveur de statistiques sur http://localhost:8081")
	fmt.Println("Appuyez sur Ctrl+C pour arrêter le serveur")

	// Initialiser le router
	mux := http.NewServeMux()

	// Route principale pour afficher les statistiques
	mux.HandleFunc("/", handleStatsPage)

	// Route API pour obtenir les données JSON pour les graphiques
	mux.HandleFunc("/api/stats", handleStatsAPI)

	// Route API pour les données de comparaison d'exchanges
	mux.HandleFunc("/api/exchanges-comparison", handleExchangesComparisonAPI)

	// Route API pour les données de performance par période
	mux.HandleFunc("/api/period-performance", handlePeriodPerformanceAPI)

	// Route API pour les données d'accumulation
	mux.HandleFunc("/api/accumulation-stats", handleAccumulationStatsAPI)

	// Démarrer le serveur sur un port différent pour éviter les conflits
	err := http.ListenAndServe("localhost:8081", mux)
	if err != nil {
		log.Fatal(err)
	}
}

// Structure pour les statistiques globales
type GlobalStats struct {
	TotalCycles          int       `json:"totalCycles"`
	CompletedCycles      int       `json:"completedCycles"`
	BuyCycles            int       `json:"buyCycles"`
	SellCycles           int       `json:"sellCycles"`
	TotalBuyVolume       float64   `json:"totalBuyVolume"`
	TotalSellVolume      float64   `json:"totalSellVolume"`
	TotalProfit          float64   `json:"totalProfit"`
	ProfitPercentage     float64   `json:"profitPercentage"`
	AverageCycleDuration float64   `json:"averageCycleDuration"` // En heures
	SuccessRate          float64   `json:"successRate"`          // % de cycles complétés avec profit
	LastUpdate           time.Time `json:"lastUpdate"`
}

// Structure pour les statistiques par exchange
type ExchangeStats struct {
	Name                 string  `json:"name"`
	TotalCycles          int     `json:"totalCycles"`
	CompletedCycles      int     `json:"completedCycles"`
	BuyCycles            int     `json:"buyCycles"`
	SellCycles           int     `json:"sellCycles"`
	TotalBuyVolume       float64 `json:"totalBuyVolume"`
	TotalSellVolume      float64 `json:"totalSellVolume"`
	TotalProfit          float64 `json:"totalProfit"`
	ProfitPercentage     float64 `json:"profitPercentage"`
	AverageCycleDuration float64 `json:"averageCycleDuration"` // En heures
	SuccessRate          float64 `json:"successRate"`          // % de cycles complétés avec profit
	AccumulationCount    int     `json:"accumulationCount"`
	AccumulatedBTC       float64 `json:"accumulatedBTC"`
}

// Structure pour les statistiques de performance temporelle
type PerformanceStats struct {
	Period       string    `json:"period"` // ex: "7j", "30j", "90j", etc.
	StartDate    time.Time `json:"startDate"`
	EndDate      time.Time `json:"endDate"`
	TotalCycles  int       `json:"totalCycles"`
	TotalProfit  float64   `json:"totalProfit"`
	SuccessRate  float64   `json:"successRate"`
	VolumeTraded float64   `json:"volumeTraded"`
}

// Structure pour les données de profitabilité temporelle
type ProfitTimePoint struct {
	Date     time.Time `json:"date"`
	Profit   float64   `json:"profit"`
	Exchange string    `json:"exchange"`
}

// Structure pour les données journalières
type DailyProfitData struct {
	Date   string  `json:"date"`
	Profit float64 `json:"profit"`
}

// handleStatsPage gère l'affichage de la page de statistiques avancées
func handleStatsPage(w http.ResponseWriter, r *http.Request) {
	// Définir le template HTML avec le support des graphiques
	statsTemplate := `<!DOCTYPE html>
<html lang="fr">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Cryptomancien - Statistiques Avancées</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.3/dist/css/bootstrap.min.css">
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/moment@2.29.4/moment.min.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/chartjs-adapter-moment@1.0.1/dist/chartjs-adapter-moment.min.js"></script>
    <style>
        body {
            padding-top: 20px;
            background-color: #f8f9fa;
        }
        .header {
            margin-bottom: 30px;
        }
        .stats-card {
            margin-bottom: 20px;
            transition: transform 0.3s;
            height: 100%;
        }
        .stats-card:hover {
            transform: translateY(-5px);
        }
        .profit-positive {
            color: #28a745;
        }
        .profit-negative {
            color: #dc3545;
        }
        .chart-container {
            position: relative;
            height: 400px;
            width: 100%;
            margin-bottom: 30px;
        }
        .period-selector {
            margin-bottom: 20px;
        }
        .nav-tabs .nav-link {
            cursor: pointer;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1 class="text-center mb-4">Cryptomancien - Statistiques Avancées</h1>
            <div class="row">
                <div class="col-md-12">
                    <div class="card">
                        <div class="card-body">
                            <div class="period-selector d-flex justify-content-center">
                                <div class="btn-group" role="group">
                                    <button type="button" class="btn btn-outline-primary" data-period="7j">7 jours</button>
                                    <button type="button" class="btn btn-outline-primary" data-period="30j">30 jours</button>
                                    <button type="button" class="btn btn-outline-primary" data-period="90j">3 mois</button>
                                    <button type="button" class="btn btn-outline-primary" data-period="180j">6 mois</button>
                                    <button type="button" class="btn btn-outline-primary" data-period="365j">1 an</button>
                                    <button type="button" class="btn btn-outline-primary active" data-period="all">Tout</button>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <!-- Statistiques globales -->
        <div class="row mb-4">
            <div class="col-12">
                <h2 class="mb-3">Statistiques Globales</h2>
            </div>
            <div class="col-md-3">
                <div class="card stats-card bg-light">
                    <div class="card-body text-center">
                        <h5 class="card-title">Cycles Totaux</h5>
                        <p class="card-text fs-2" id="total-cycles">-</p>
                    </div>
                </div>
            </div>
            <div class="col-md-3">
                <div class="card stats-card bg-primary text-white">
                    <div class="card-body text-center">
                        <h5 class="card-title">Cycles Complétés</h5>
                        <p class="card-text fs-2" id="completed-cycles">-</p>
                    </div>
                </div>
            </div>
            <div class="col-md-3">
                <div class="card stats-card bg-light">
                    <div class="card-body text-center">
                        <h5 class="card-title">Volume Total</h5>
                        <p class="card-text fs-2" id="total-volume">-</p>
                    </div>
                </div>
            </div>
            <div class="col-md-3">
                <div class="card stats-card bg-success text-white">
                    <div class="card-body text-center">
                        <h5 class="card-title">Profit Total</h5>
                        <p class="card-text fs-2" id="total-profit">-</p>
                    </div>
                </div>
            </div>
        </div>

        <div class="row mb-4">
            <div class="col-md-4">
                <div class="card stats-card">
                    <div class="card-body text-center">
                        <h5 class="card-title">Taux de Réussite</h5>
                        <p class="card-text fs-2" id="success-rate">-</p>
                    </div>
                </div>
            </div>
            <div class="col-md-4">
                <div class="card stats-card">
                    <div class="card-body text-center">
                        <h5 class="card-title">Durée Moyenne du Cycle</h5>
                        <p class="card-text fs-2" id="avg-duration">-</p>
                    </div>
                </div>
            </div>
            <div class="col-md-4">
                <div class="card stats-card">
                    <div class="card-body text-center">
                        <h5 class="card-title">Rentabilité Moyenne</h5>
                        <p class="card-text fs-2" id="avg-profitability">-</p>
                    </div>
                </div>
            </div>
        </div>

        <!-- Navigation par onglets -->
        <ul class="nav nav-tabs" id="myTab" role="tablist">
            <li class="nav-item" role="presentation">
                <button class="nav-link active" id="profit-history-tab" data-bs-toggle="tab" data-bs-target="#profit-history" type="button" role="tab">Historique des Profits</button>
            </li>
            <li class="nav-item" role="presentation">
                <button class="nav-link" id="exchange-comparison-tab" data-bs-toggle="tab" data-bs-target="#exchange-comparison" type="button" role="tab">Comparaison des Exchanges</button>
            </li>
            <li class="nav-item" role="presentation">
                <button class="nav-link" id="period-performance-tab" data-bs-toggle="tab" data-bs-target="#period-performance" type="button" role="tab">Performance par Période</button>
            </li>
            <li class="nav-item" role="presentation">
                <button class="nav-link" id="accumulation-tab" data-bs-toggle="tab" data-bs-target="#accumulation" type="button" role="tab">Accumulation</button>
            </li>
        </ul>

        <!-- Contenu des onglets -->
        <div class="tab-content mt-4" id="myTabContent">
            <!-- Onglet Historique des Profits -->
            <div class="tab-pane fade show active" id="profit-history" role="tabpanel">
                <div class="chart-container">
                    <canvas id="profit-history-chart"></canvas>
                </div>
                <div class="chart-container">
                    <canvas id="daily-profit-chart"></canvas>
                </div>
            </div>
            
            <!-- Onglet Comparaison des Exchanges -->
            <div class="tab-pane fade" id="exchange-comparison" role="tabpanel">
                <div class="row">
                    <div class="col-md-6">
                        <div class="chart-container">
                            <canvas id="exchange-profit-chart"></canvas>
                        </div>
                    </div>
                    <div class="col-md-6">
                        <div class="chart-container">
                            <canvas id="exchange-volume-chart"></canvas>
                        </div>
                    </div>
                </div>
                <div class="row mt-4">
                    <div class="col-md-6">
                        <div class="chart-container">
                            <canvas id="exchange-success-chart"></canvas>
                        </div>
                    </div>
                    <div class="col-md-6">
                        <div class="chart-container">
                            <canvas id="exchange-duration-chart"></canvas>
                        </div>
                    </div>
                </div>
            </div>
            
            <!-- Onglet Performance par Période -->
            <div class="tab-pane fade" id="period-performance" role="tabpanel">
                <div class="row">
                    <div class="col-md-6">
                        <div class="chart-container">
                            <canvas id="period-profit-chart"></canvas>
                        </div>
                    </div>
                    <div class="col-md-6">
                        <div class="chart-container">
                            <canvas id="period-success-chart"></canvas>
                        </div>
                    </div>
                </div>
            </div>
            
            <!-- Onglet Accumulation -->
            <div class="tab-pane fade" id="accumulation" role="tabpanel">
                <div class="row">
                    <div class="col-md-6">
                        <div class="chart-container">
                            <canvas id="accumulation-volume-chart"></canvas>
                        </div>
                    </div>
                    <div class="col-md-6">
                        <div class="chart-container">
                            <canvas id="accumulation-savings-chart"></canvas>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <div class="mt-4 text-muted">
            <p>Dernière mise à jour: <span id="last-update"></span></p>
            <p><a href="/" class="btn btn-outline-secondary">Retour au tableau de bord principal</a></p>
        </div>
    </div>

    <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.3/dist/js/bootstrap.bundle.min.js"></script>
    <script>
        // Fonction pour formater les durées
        function formatDuration(hours) {
            if (hours < 1) {
                return Math.round(hours * 60) + ' min';
            } else if (hours < 24) {
                const h = Math.floor(hours);
                const m = Math.round((hours - h) * 60);
                return h + 'h ' + (m > 0 ? m + 'm' : '');
            } else {
                const days = Math.floor(hours / 24);
                const h = Math.floor(hours % 24);
                return days + 'j ' + (h > 0 ? h + 'h' : '');
            }
        }

        // Fonction pour charger les statistiques globales
        async function loadGlobalStats(period = 'all') {
            try {
                const response = await fetch('/api/stats?period=' + period);
                const data = await response.json();
                
                // Mettre à jour les cartes de statistiques
                document.getElementById('total-cycles').textContent = data.totalCycles;
                document.getElementById('completed-cycles').textContent = data.completedCycles;
                document.getElementById('total-volume').textContent = data.totalBuyVolume.toFixed(2) + ' USDC';
                
                const profitElement = document.getElementById('total-profit');
                profitElement.textContent = data.totalProfit.toFixed(2) + ' USDC (' + data.profitPercentage.toFixed(2) + '%)';
                profitElement.className = data.totalProfit >= 0 ? 'card-text fs-2' : 'card-text fs-2 text-danger';
                
                document.getElementById('success-rate').textContent = data.successRate.toFixed(2) + '%';
                document.getElementById('avg-duration').textContent = formatDuration(data.averageCycleDuration);
                document.getElementById('avg-profitability').textContent = data.profitPercentage.toFixed(2) + '%';
                
                document.getElementById('last-update').textContent = new Date().toLocaleString();
                
                // Charger les graphiques
                loadProfitHistoryChart(period);
                loadDailyProfitChart(period);
            } catch (error) {
                console.error('Erreur lors du chargement des statistiques:', error);
            }
        }

        // Fonction pour charger le graphique d'historique des profits
        async function loadProfitHistoryChart(period = 'all') {
            try {
                const response = await fetch('/api/stats?period=' + period);
                const globalData = await response.json();
                
                // Récupérer les données de l'historique des profits
                const profitPoints = globalData.profitHistory || [];
                
                // Créer des ensembles de données par exchange
                const exchanges = [...new Set(profitPoints.map(point => point.exchange))];
                const datasets = exchanges.map((exchange, index) => {
                    const colors = ['#28a745', '#007bff', '#fd7e14', '#6f42c1', '#e83e8c'];
                    return {
                        label: exchange,
                        data: profitPoints
                            .filter(point => point.exchange === exchange)
                            .map(point => ({
                                x: new Date(point.date),
                                y: point.profit
                            })),
                        borderColor: colors[index % colors.length],
                        backgroundColor: colors[index % colors.length] + '33',
                        fill: false,
                        tension: 0.1
                    };
                });
                
                // Créer le graphique
                const ctx = document.getElementById('profit-history-chart').getContext('2d');
                
                // Détruire le graphique existant s'il existe
                if (window.profitHistoryChart) {
                    window.profitHistoryChart.destroy();
                }
                
                window.profitHistoryChart = new Chart(ctx, {
                    type: 'line',
                    data: {
                        datasets: datasets
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: {
                            title: {
                                display: true,
                                text: 'Évolution du Profit par Exchange au fil du temps',
                                font: {
                                    size: 16
                                }
                            },
                            tooltip: {
                                mode: 'index',
                                intersect: false
                            },
                            legend: {
                                position: 'top'
                            }
                        },
                        scales: {
                            x: {
                                type: 'time',
                                time: {
                                    unit: 'day',
                                    tooltipFormat: 'DD MMM YYYY'
                                },
                                title: {
                                    display: true,
                                    text: 'Date'
                                }
                            },
                            y: {
                                title: {
                                    display: true,
                                    text: 'Profit (USDC)'
                                }
                            }
                        }
                    }
                });
            } catch (error) {
                console.error('Erreur lors du chargement du graphique d\'historique des profits:', error);
            }
        }

        // Fonction pour charger le graphique des profits journaliers
        async function loadDailyProfitChart(period = 'all') {
            try {
                const response = await fetch('/api/stats?period=' + period);
                const globalData = await response.json();
                
                // Récupérer les données des profits journaliers
                const dailyProfits = globalData.dailyProfits || [];
                
                // Créer le graphique
                const ctx = document.getElementById('daily-profit-chart').getContext('2d');
                
                // Détruire le graphique existant s'il existe
                if (window.dailyProfitChart) {
                    window.dailyProfitChart.destroy();
                }
                
                window.dailyProfitChart = new Chart(ctx, {
                    type: 'bar',
                    data: {
                        labels: dailyProfits.map(day => day.date),
                        datasets: [{
                            label: 'Profit Journalier',
                            data: dailyProfits.map(day => day.profit),
                            backgroundColor: function(context) {
                                const value = context.dataset.data[context.dataIndex];
                                return value >= 0 ? 'rgba(40, 167, 69, 0.6)' : 'rgba(220, 53, 69, 0.6)';
                            },
                            borderColor: function(context) {
                                const value = context.dataset.data[context.dataIndex];
                                return value >= 0 ? 'rgb(40, 167, 69)' : 'rgb(220, 53, 69)';
                            },
                            borderWidth: 1
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: {
                            title: {
                                display: true,
                                text: 'Profits Journaliers',
                                font: {
                                    size: 16
                                }
                            },
                            legend: {
                                display: false
                            }
                        },
                        scales: {
                            x: {
                                title: {
                                    display: true,
                                    text: 'Date'
                                }
                            },
                            y: {
                                title: {
                                    display: true,
                                    text: 'Profit (USDC)'
                                }
                            }
                        }
                    }
                });
            } catch (error) {
                console.error('Erreur lors du chargement du graphique des profits journaliers:', error);
            }
        }

        // Fonction pour charger les graphiques de comparaison d'exchanges
        async function loadExchangeComparisonCharts(period = 'all') {
            try {
                const response = await fetch('/api/exchanges-comparison?period=' + period);
                const data = await response.json();
                
                const exchangeNames = data.map(exchange => exchange.name);
                const profits = data.map(exchange => exchange.totalProfit);
                const volumes = data.map(exchange => exchange.totalBuyVolume);
                const successRates = data.map(exchange => exchange.successRate);
                const durations = data.map(exchange => exchange.averageCycleDuration);
                
                // Graphique de comparaison des profits par exchange
                createExchangeComparisonChart('exchange-profit-chart', exchangeNames, profits, 'Profit Total par Exchange', 'Profit (USDC)', 'bar');
                
                // Graphique de comparaison des volumes par exchange
                createExchangeComparisonChart('exchange-volume-chart', exchangeNames, volumes, 'Volume Total par Exchange', 'Volume (USDC)', 'bar');
                
                // Graphique de comparaison des taux de réussite par exchange
                createExchangeComparisonChart('exchange-success-chart', exchangeNames, successRates, 'Taux de Réussite par Exchange', 'Taux de Réussite (%)', 'bar');
                
                // Graphique de comparaison des durées moyennes de cycle par exchange
                createExchangeComparisonChart('exchange-duration-chart', exchangeNames, durations, 'Durée Moyenne des Cycles par Exchange', 'Durée (heures)', 'bar');
            } catch (error) {
                console.error('Erreur lors du chargement des graphiques de comparaison d\'exchanges:', error);
            }
        }

        // Fonction pour créer un graphique de comparaison d'exchanges
        function createExchangeComparisonChart(canvasId, labels, data, title, yAxisTitle, type = 'bar') {
            const colors = ['#28a745', '#007bff', '#fd7e14', '#6f42c1', '#e83e8c'];
            
            const ctx = document.getElementById(canvasId).getContext('2d');
            
            // Détruire le graphique existant s'il existe
            if (window[canvasId + 'Chart']) {
                window[canvasId + 'Chart'].destroy();
            }
            
            window[canvasId + 'Chart'] = new Chart(ctx, {
                type: type,
                data: {
                    labels: labels,
                    datasets: [{
                        label: title,
                        data: data,
                        backgroundColor: colors.map(color => color + '80'),
                        borderColor: colors,
                        borderWidth: 1
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: title,
                            font: {
                                size: 16
                            }
                        },
                        legend: {
                            display: false
                        }
                    },
                    scales: {
                        y: {
                            title: {
                                display: true,
                                text: yAxisTitle
                            }
                        }
                    }
                }
            });
        }

        // Fonction pour charger les graphiques de performance par période
        async function loadPeriodPerformanceCharts(period = 'all') {
            try {
                const response = await fetch('/api/period-performance?period=' + period);
                const data = await response.json();
                
                const periods = data.map(period => period.period);
                const profits = data.map(period => period.totalProfit);
                const successRates = data.map(period => period.successRate);
                
                // Graphique de profit par période
                createPeriodPerformanceChart('period-profit-chart', periods, profits, 'Profit Total par Période', 'Profit (USDC)');
                
                // Graphique de taux de réussite par période
                createPeriodPerformanceChart('period-success-chart', periods, successRates, 'Taux de Réussite par Période', 'Taux de Réussite (%)');
            } catch (error) {
                console.error('Erreur lors du chargement des graphiques de performance par période:', error);
            }
        }

        // Fonction pour créer un graphique de performance par période
        function createPeriodPerformanceChart(canvasId, labels, data, title, yAxisTitle) {
            const ctx = document.getElementById(canvasId).getContext('2d');
            
            // Détruire le graphique existant s'il existe
            if (window[canvasId + 'Chart']) {
                window[canvasId + 'Chart'].destroy();
            }
            
            window[canvasId + 'Chart'] = new Chart(ctx, {
                type: 'line',
                data: {
                    labels: labels,
                    datasets: [{
                        label: title,
                        data: data,
                        backgroundColor: 'rgba(40, 167, 69, 0.2)',
                        borderColor: 'rgb(40, 167, 69)',
                        borderWidth: 2,
                        fill: true,
                        tension: 0.1
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: title,
                            font: {
                                size: 16
                            }
                        },
                        legend: {
                            display: false
                        }
                    },
                    scales: {
                        y: {
                            title: {
                                display: true,
                                text: yAxisTitle
                            }
                        }
                    }
                }
            });
        }

        // Fonction pour charger les graphiques d'accumulation
        async function loadAccumulationCharts(period = 'all') {
            try {
                const response = await fetch('/api/accumulation-stats?period=' + period);
                const data = await response.json();
                
                const exchangeNames = data.map(exchange => exchange.name);
                const btcVolumes = data.map(exchange => exchange.accumulatedBTC);
                const savingsValues = data.map(exchange => exchange.savedValue);
                
                // Graphique des volumes de BTC accumulés
                createAccumulationChart('accumulation-volume-chart', exchangeNames, btcVolumes, 'Volume BTC Accumulé par Exchange', 'BTC');
                
                // Graphique des économies réalisées grâce à l'accumulation
                createAccumulationChart('accumulation-savings-chart', exchangeNames, savingsValues, 'Économies Réalisées par Exchange', 'USDC');
            } catch (error) {
                console.error('Erreur lors du chargement des graphiques d\'accumulation:', error);
            }
        }

        // Fonction pour créer un graphique d'accumulation
        function createAccumulationChart(canvasId, labels, data, title, yAxisTitle) {
            const colors = ['#28a745', '#007bff', '#fd7e14', '#6f42c1', '#e83e8c'];
            
            const ctx = document.getElementById(canvasId).getContext('2d');
            
            // Détruire le graphique existant s'il existe
            if (window[canvasId + 'Chart']) {
                window[canvasId + 'Chart'].destroy();
            }
            
            window[canvasId + 'Chart'] = new Chart(ctx, {
                type: 'bar',
                data: {
                    labels: labels,
                    datasets: [{
                        label: title,
                        data: data,
                        backgroundColor: colors.map(color => color + '80'),
                        borderColor: colors,
                        borderWidth: 1
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        title: {
                            display: true,
                            text: title,
                            font: {
                                size: 16
                            }
                        },
                        legend: {
                            display: false
                        }
                    },
                    scales: {
                        y: {
                            title: {
                                display: true,
                                text: yAxisTitle
                            }
                        }
                    }
                }
            });
        }

        // Une fois que tout est chargé
        document.addEventListener('DOMContentLoaded', function() {
            // Charger les statistiques initiales avec tous les données
            loadGlobalStats('all');
            
            // Charger les différents graphiques
            loadExchangeComparisonCharts('all');
            loadPeriodPerformanceCharts('all');
            loadAccumulationCharts('all');
            
            // Gestion des sélecteurs de période
            document.querySelectorAll('.period-selector button').forEach(button => {
                button.addEventListener('click', function() {
                    // Mettre à jour la classe active
                    document.querySelectorAll('.period-selector button').forEach(btn => {
                        btn.classList.remove('active');
                    });
                    this.classList.add('active');
                    
                    // Récupérer la période sélectionnée
                    const period = this.getAttribute('data-period');
                    
                    // Charger les données pour cette période
                    loadGlobalStats(period);
                    loadExchangeComparisonCharts(period);
                    loadPeriodPerformanceCharts(period);
                    loadAccumulationCharts(period);
                });
            });
        });
    </script>
</body>
</html>`

	// Exécuter le template
	tmpl, err := template.New("statsPage").Parse(statsTemplate)
	if err != nil {
		http.Error(w, "Erreur lors de la compilation du template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Données à passer au template
	data := map[string]interface{}{}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Erreur lors du rendu du template: "+err.Error(), http.StatusInternalServerError)
	}
}

// handleStatsAPI gère les requêtes API pour les statistiques globales et historiques
func handleStatsAPI(w http.ResponseWriter, r *http.Request) {
	// Récupérer le paramètre de période
	period := r.URL.Query().Get("period")

	// Calculer les dates de début et de fin en fonction de la période
	startDate, endDate := calculateDateRangeFromPeriod(period)

	// Récupérer tous les cycles
	repo := database.GetRepository()
	allCycles, err := repo.FindAll()
	if err != nil {
		http.Error(w, "Erreur lors de la récupération des cycles: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Filtrer les cycles en fonction de la période
	var filteredCycles []*database.Cycle
	for _, cycle := range allCycles {
		if (startDate == nil || !cycle.CreatedAt.Before(*startDate)) &&
			(endDate == nil || !cycle.CreatedAt.After(*endDate)) {
			filteredCycles = append(filteredCycles, cycle)
		}
	}

	// Calculer les statistiques globales
	stats := calculateGlobalStats(filteredCycles)

	// Ajouter l'historique des profits
	profitHistory := calculateProfitHistory(filteredCycles)
	stats.ProfitHistory = profitHistory

	// Ajouter les profits journaliers
	dailyProfits := calculateDailyProfits(filteredCycles)
	stats.DailyProfits = dailyProfits

	// Retourner les statistiques au format JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleExchangesComparisonAPI gère les requêtes API pour les données de comparaison d'exchanges
func handleExchangesComparisonAPI(w http.ResponseWriter, r *http.Request) {
	// Récupérer le paramètre de période
	period := r.URL.Query().Get("period")

	// Calculer les dates de début et de fin en fonction de la période
	startDate, endDate := calculateDateRangeFromPeriod(period)

	// Récupérer tous les cycles
	repo := database.GetRepository()
	allCycles, err := repo.FindAll()
	if err != nil {
		http.Error(w, "Erreur lors de la récupération des cycles: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Filtrer les cycles en fonction de la période
	var filteredCycles []*database.Cycle
	for _, cycle := range allCycles {
		if (startDate == nil || !cycle.CreatedAt.Before(*startDate)) &&
			(endDate == nil || !cycle.CreatedAt.After(*endDate)) {
			filteredCycles = append(filteredCycles, cycle)
		}
	}

	// Calculer les statistiques par exchange
	exchangeStats := calculateExchangeStats(filteredCycles)

	// Retourner les statistiques au format JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exchangeStats)
}

// handlePeriodPerformanceAPI gère les requêtes API pour les données de performance par période
func handlePeriodPerformanceAPI(w http.ResponseWriter, r *http.Request) {
	// Récupérer le paramètre de période globale
	globalPeriod := r.URL.Query().Get("period")

	// Calculer les dates de début et de fin en fonction de la période globale
	startDate, endDate := calculateDateRangeFromPeriod(globalPeriod)

	// Récupérer tous les cycles
	repo := database.GetRepository()
	allCycles, err := repo.FindAll()
	if err != nil {
		http.Error(w, "Erreur lors de la récupération des cycles: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Filtrer les cycles en fonction de la période globale
	var filteredCycles []*database.Cycle
	for _, cycle := range allCycles {
		if (startDate == nil || !cycle.CreatedAt.Before(*startDate)) &&
			(endDate == nil || !cycle.CreatedAt.After(*endDate)) {
			filteredCycles = append(filteredCycles, cycle)
		}
	}

	// Définir les périodes d'analyse
	periods := []string{"7j", "30j", "90j", "180j", "365j"}

	// Calculer les statistiques pour chaque période
	periodStats := make([]PerformanceStats, 0, len(periods))
	now := time.Now()

	for _, p := range periods {
		pStartDate, _ := calculateDateRangeFromPeriod(p)
		if pStartDate != nil {
			// Filtrer les cycles pour cette période spécifique
			var periodCycles []*database.Cycle
			for _, cycle := range filteredCycles {
				if !cycle.CreatedAt.Before(*pStartDate) {
					periodCycles = append(periodCycles, cycle)
				}
			}

			// Calculer les statistiques pour cette période
			totalCycles := len(periodCycles)
			var totalProfit float64
			var successCount int
			var volumeTraded float64

			for _, cycle := range periodCycles {
				if cycle.Status == "completed" {
					profit := (cycle.SellPrice - cycle.BuyPrice) * cycle.Quantity
					totalProfit += profit

					if profit > 0 {
						successCount++
					}

					volumeTraded += cycle.BuyPrice * cycle.Quantity
				}
			}

			// Calculer le taux de réussite
			successRate := 0.0
			if len(periodCycles) > 0 {
				successRate = float64(successCount) / float64(totalCycles) * 100
			}

			// Ajouter les statistiques de cette période
			periodStats = append(periodStats, PerformanceStats{
				Period:       p,
				StartDate:    *pStartDate,
				EndDate:      now,
				TotalCycles:  totalCycles,
				TotalProfit:  totalProfit,
				SuccessRate:  successRate,
				VolumeTraded: volumeTraded,
			})
		}
	}

	// Retourner les statistiques au format JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(periodStats)
}

// handleAccumulationStatsAPI gère les requêtes API pour les données d'accumulation
func handleAccumulationStatsAPI(w http.ResponseWriter, r *http.Request) {
	// Récupérer le paramètre de période
	period := r.URL.Query().Get("period")

	// Calculer les dates de début et de fin en fonction de la période
	startDate, endDate := calculateDateRangeFromPeriod(period)

	// Récupérer le repository d'accumulations
	accuRepo := database.GetAccumulationRepository()

	// Récupérer toutes les accumulations
	allAccumulations, err := accuRepo.FindAll()
	if err != nil {
		http.Error(w, "Erreur lors de la récupération des accumulations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Filtrer les accumulations en fonction de la période
	var filteredAccumulations []*database.Accumulation
	for _, accu := range allAccumulations {
		if (startDate == nil || !accu.CreatedAt.Before(*startDate)) &&
			(endDate == nil || !accu.CreatedAt.After(*endDate)) {
			filteredAccumulations = append(filteredAccumulations, accu)
		}
	}

	// Récupérer la configuration pour obtenir la liste des exchanges
	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, "Erreur lors du chargement de la configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Calculer les statistiques d'accumulation par exchange
	accuStats := make([]map[string]interface{}, 0)

	for exchangeName, exchangeConfig := range cfg.Exchanges {
		if exchangeConfig.Enabled {
			// Filtrer les accumulations pour cet exchange
			var exchangeAccu []*database.Accumulation
			for _, accu := range filteredAccumulations {
				if accu.Exchange == exchangeName {
					exchangeAccu = append(exchangeAccu, accu)
				}
			}

			// Calculer les statistiques pour cet exchange
			accumulatedBTC := 0.0
			savedValue := 0.0

			for _, accu := range exchangeAccu {
				accumulatedBTC += accu.Quantity

				// Calcul de la valeur économisée (différence entre le prix de vente cible et le prix d'annulation)
				savedPerBTC := accu.TargetSellPrice - accu.CancelPrice
				savedValue += savedPerBTC * accu.Quantity
			}

			// Ajouter les statistiques de cet exchange
			accuStats = append(accuStats, map[string]interface{}{
				"name":           exchangeName,
				"enabled":        exchangeConfig.Accumulation,
				"count":          len(exchangeAccu),
				"accumulatedBTC": accumulatedBTC,
				"savedValue":     savedValue,
			})
		}
	}

	// Retourner les statistiques au format JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accuStats)
}

// Structure complète pour les statistiques globales avec historique
type CompleteGlobalStats struct {
	GlobalStats
	ProfitHistory []ProfitTimePoint `json:"profitHistory"`
	DailyProfits  []DailyProfitData `json:"dailyProfits"`
}

// Calcule les statistiques globales pour un ensemble de cycles
func calculateGlobalStats(cycles []*database.Cycle) CompleteGlobalStats {
	var stats CompleteGlobalStats

	// Initialiser les compteurs
	stats.TotalCycles = len(cycles)
	stats.CompletedCycles = 0
	stats.BuyCycles = 0
	stats.SellCycles = 0
	stats.TotalBuyVolume = 0
	stats.TotalSellVolume = 0
	stats.TotalProfit = 0

	var totalDuration float64
	var profitableCycles int

	// Calculer les statistiques
	for _, cycle := range cycles {
		switch cycle.Status {
		case "buy":
			stats.BuyCycles++
		case "sell":
			stats.SellCycles++
		case "completed":
			stats.CompletedCycles++

			// Calculer les volumes et profits
			buyVolume := cycle.BuyPrice * cycle.Quantity
			sellVolume := cycle.SellPrice * cycle.Quantity
			profit := sellVolume - buyVolume

			stats.TotalBuyVolume += buyVolume
			stats.TotalSellVolume += sellVolume
			stats.TotalProfit += profit

			// Calculer la durée du cycle
			var duration float64
			if !cycle.CompletedAt.IsZero() {
				duration = cycle.CompletedAt.Sub(cycle.CreatedAt).Hours()
			} else {
				// Estimer la durée si la date de complétion n'est pas définie
				switch cycle.Exchange {
				case "KUCOIN":
					duration = 6
				case "MEXC":
					duration = 2
				case "BINANCE":
					duration = 4
				case "KRAKEN":
					duration = 4
				default:
					duration = 3
				}
			}

			totalDuration += duration

			// Compter les cycles profitables
			if profit > 0 {
				profitableCycles++
			}
		}
	}

	// Calculer les statistiques dérivées
	if stats.CompletedCycles > 0 {
		stats.AverageCycleDuration = totalDuration / float64(stats.CompletedCycles)
		stats.SuccessRate = float64(profitableCycles) / float64(stats.CompletedCycles) * 100
	}

	if stats.TotalBuyVolume > 0 {
		stats.ProfitPercentage = stats.TotalProfit / stats.TotalBuyVolume * 100
	}

	stats.LastUpdate = time.Now()

	return stats
}

// Calcule les statistiques par exchange
func calculateExchangeStats(cycles []*database.Cycle) []ExchangeStats {
	// Créer une map pour stocker les statistiques par exchange
	statsMap := make(map[string]*ExchangeStats)

	// Récupérer le repository d'accumulations
	accuRepo := database.GetAccumulationRepository()

	// Initialiser les statistiques pour chaque exchange présent dans les cycles
	for _, cycle := range cycles {
		if _, exists := statsMap[cycle.Exchange]; !exists {
			statsMap[cycle.Exchange] = &ExchangeStats{
				Name: cycle.Exchange,
			}

			// Récupérer le nombre d'accumulations pour cet exchange
			count, err := accuRepo.CountByExchange(cycle.Exchange)
			if err == nil {
				statsMap[cycle.Exchange].AccumulationCount = count
			}

			// Récupérer le total de BTC accumulé pour cet exchange
			accumulatedBTC, err := accuRepo.GetTotalAccumulatedBTC(cycle.Exchange)
			if err == nil {
				statsMap[cycle.Exchange].AccumulatedBTC = accumulatedBTC
			}
		}
	}

	// Calculer les statistiques pour chaque cycle
	for _, cycle := range cycles {
		stats := statsMap[cycle.Exchange]

		stats.TotalCycles++

		switch cycle.Status {
		case "buy":
			stats.BuyCycles++
		case "sell":
			stats.SellCycles++
		case "completed":
			stats.CompletedCycles++

			// Calculer les volumes et profits
			buyVolume := cycle.BuyPrice * cycle.Quantity
			sellVolume := cycle.SellPrice * cycle.Quantity
			profit := sellVolume - buyVolume

			stats.TotalBuyVolume += buyVolume
			stats.TotalSellVolume += sellVolume
			stats.TotalProfit += profit

			// Calculer la durée du cycle
			var duration float64
			if !cycle.CompletedAt.IsZero() {
				duration = cycle.CompletedAt.Sub(cycle.CreatedAt).Hours()
			} else {
				// Estimer la durée si la date de complétion n'est pas définie
				switch cycle.Exchange {
				case "KUCOIN":
					duration = 6
				case "MEXC":
					duration = 2
				case "BINANCE":
					duration = 4
				default:
					duration = 3
				}
			}

			stats.AverageCycleDuration += duration

			// Compter les cycles profitables
			if profit > 0 {
				stats.SuccessRate++
			}
		}
	}

	// Calculer les statistiques moyennes et pourcentages
	for _, stats := range statsMap {
		if stats.CompletedCycles > 0 {
			stats.AverageCycleDuration /= float64(stats.CompletedCycles)
			stats.SuccessRate = (stats.SuccessRate / float64(stats.CompletedCycles)) * 100
		}

		if stats.TotalBuyVolume > 0 {
			stats.ProfitPercentage = stats.TotalProfit / stats.TotalBuyVolume * 100
		}
	}

	// Convertir la map en slice pour le retour
	result := make([]ExchangeStats, 0, len(statsMap))
	for _, stats := range statsMap {
		result = append(result, *stats)
	}

	// Trier par profit total (ordre décroissant)
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalProfit > result[j].TotalProfit
	})

	return result
}

// Calcule l'historique des profits au fil du temps
func calculateProfitHistory(cycles []*database.Cycle) []ProfitTimePoint {
	// Filtrer seulement les cycles complétés
	var completedCycles []*database.Cycle
	for _, cycle := range cycles {
		if cycle.Status == "completed" {
			completedCycles = append(completedCycles, cycle)
		}
	}

	// Trier les cycles par date de complétion
	sort.Slice(completedCycles, func(i, j int) bool {
		// Utiliser la date de création si la date de complétion n'est pas définie
		dateI := completedCycles[i].CreatedAt
		if !completedCycles[i].CompletedAt.IsZero() {
			dateI = completedCycles[i].CompletedAt
		}

		dateJ := completedCycles[j].CreatedAt
		if !completedCycles[j].CompletedAt.IsZero() {
			dateJ = completedCycles[j].CompletedAt
		}

		return dateI.Before(dateJ)
	})

	// Créer les points de profit cumulé par exchange
	pointsByExchange := make(map[string][]ProfitTimePoint)
	cumulativeProfitByExchange := make(map[string]float64)

	for _, cycle := range completedCycles {
		// Calculer le profit de ce cycle
		profit := (cycle.SellPrice - cycle.BuyPrice) * cycle.Quantity

		// Cumuler le profit pour cet exchange
		cumulativeProfitByExchange[cycle.Exchange] += profit

		// Déterminer la date à utiliser (date de complétion ou date de création)
		date := cycle.CreatedAt
		if !cycle.CompletedAt.IsZero() {
			date = cycle.CompletedAt
		}

		// Ajouter un point de données pour cet exchange
		pointsByExchange[cycle.Exchange] = append(pointsByExchange[cycle.Exchange], ProfitTimePoint{
			Date:     date,
			Profit:   cumulativeProfitByExchange[cycle.Exchange],
			Exchange: cycle.Exchange,
		})
	}

	// Aplatir la structure pour le retour
	var result []ProfitTimePoint
	for _, points := range pointsByExchange {
		result = append(result, points...)
	}

	// Trier tous les points par date
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date.Before(result[j].Date)
	})

	return result
}

// Calcule les profits journaliers
func calculateDailyProfits(cycles []*database.Cycle) []DailyProfitData {
	// Filtrer seulement les cycles complétés
	var completedCycles []*database.Cycle
	for _, cycle := range cycles {
		if cycle.Status == "completed" {
			completedCycles = append(completedCycles, cycle)
		}
	}

	// Map pour agréger les profits par jour
	dailyProfits := make(map[string]float64)

	for _, cycle := range completedCycles {
		// Calculer le profit de ce cycle
		profit := (cycle.SellPrice - cycle.BuyPrice) * cycle.Quantity

		// Déterminer la date à utiliser (date de complétion ou date de création)
		date := cycle.CreatedAt
		if !cycle.CompletedAt.IsZero() {
			date = cycle.CompletedAt
		}

		// Formater la date au format YYYY-MM-DD
		dateKey := date.Format("2006-01-02")

		// Ajouter le profit à ce jour
		dailyProfits[dateKey] += profit
	}

	// Convertir la map en slice
	var result []DailyProfitData
	for date, profit := range dailyProfits {
		result = append(result, DailyProfitData{
			Date:   date,
			Profit: profit,
		})
	}

	// Trier par date
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date < result[j].Date
	})

	return result
}

// Calcule la plage de dates en fonction d'une période spécifiée
func calculateDateRangeFromPeriod(period string) (*time.Time, *time.Time) {
	now := time.Now()
	end := now

	// Si aucune période n'est spécifiée ou si la période est "all", retourner nil pour indiquer aucune restriction
	if period == "" || period == "all" {
		return nil, nil
	}

	var start time.Time
	switch period {
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

	return &start, &end
}
