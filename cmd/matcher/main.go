package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ehdc-llpg/internal/db"
	"github.com/ehdc-llpg/internal/engine"
	import_pkg "github.com/ehdc-llpg/internal/import"
)

var (
	// Global database connection
	dbConn *db.Connection
)

func main() {
	var err error
	
	// Initialize database connection
	dbConn, err = db.NewConnection()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbConn.Close()

	// Create root command
	rootCmd := &cobra.Command{
		Use:   "matcher",
		Short: "EHDC LLPG Address Matching System",
		Long:  `A sophisticated address matching system for East Hampshire District Council LLPG data`,
	}

	// Add subcommands
	rootCmd.AddCommand(createImportCmd())
	rootCmd.AddCommand(createMatchCmd())
	rootCmd.AddCommand(createPingCmd())
	rootCmd.AddCommand(createDBCmd())

	// Execute root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// createPingCmd creates a command to test database connectivity
func createPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Test database connectivity",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Database connection successful!")
			
			// Show some basic info
			var count int
			err := dbConn.DB.QueryRow("SELECT COUNT(*) FROM dim_address").Scan(&count)
			if err != nil {
				log.Printf("Error counting dim_address records: %v", err)
			} else {
				fmt.Printf("LLPG addresses loaded: %d\n", count)
			}
			
			err = dbConn.DB.QueryRow("SELECT COUNT(*) FROM src_document").Scan(&count)
			if err != nil {
				log.Printf("Error counting src_document records: %v", err)
			} else {
				fmt.Printf("Source documents loaded: %d\n", count)
			}
		},
	}
}

// createImportCmd creates the import subcommand
func createImportCmd() *cobra.Command {
	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Import source document data",
		Long:  `Import CSV files containing decision notices, land charges, enforcement notices, and agreements`,
	}

	// Add import subcommands
	importCmd.AddCommand(createImportDecisionCmd())
	importCmd.AddCommand(createImportLandChargeCmd())
	importCmd.AddCommand(createImportEnforcementCmd())
	importCmd.AddCommand(createImportAgreementCmd())

	return importCmd
}

func createImportDecisionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "decision [filename]",
		Short: "Import decision notices CSV",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			filename := args[0]
			importer := import_pkg.NewCSVImporter(dbConn.DB)
			
			if err := importer.ImportDecisionNotices(filename); err != nil {
				log.Fatalf("Failed to import decision notices: %v", err)
			}
		},
	}
}

func createImportLandChargeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "land-charge [filename]",
		Short: "Import land charges cards CSV",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			filename := args[0]
			importer := import_pkg.NewCSVImporter(dbConn.DB)
			
			if err := importer.ImportLandCharges(filename); err != nil {
				log.Fatalf("Failed to import land charges: %v", err)
			}
		},
	}
}

func createImportEnforcementCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enforcement [filename]",
		Short: "Import enforcement notices CSV",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			filename := args[0]
			importer := import_pkg.NewCSVImporter(dbConn.DB)
			
			if err := importer.ImportEnforcementNotices(filename); err != nil {
				log.Fatalf("Failed to import enforcement notices: %v", err)
			}
		},
	}
}

func createImportAgreementCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agreement [filename]",
		Short: "Import agreements CSV",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			filename := args[0]
			importer := import_pkg.NewCSVImporter(dbConn.DB)
			
			if err := importer.ImportAgreements(filename); err != nil {
				log.Fatalf("Failed to import agreements: %v", err)
			}
		},
	}
}
// createMatchCmd creates the match subcommand
func createMatchCmd() *cobra.Command {
	matchCmd := &cobra.Command{
		Use:   "match",
		Short: "Run address matching algorithms",
		Long:  `Run various stages of the address matching algorithm to find UPRN matches`,
	}

	// Add match subcommands
	matchCmd.AddCommand(createMatchDeterministicCmd())
	matchCmd.AddCommand(createMatchFuzzyCmd())
	matchCmd.AddCommand(createMatchOptimizedCmd())
	matchCmd.AddCommand(createTuneThresholdsCmd())
	matchCmd.AddCommand(createMatchPostcodeCmd())
	matchCmd.AddCommand(createMatchSpatialCmd())
	matchCmd.AddCommand(createMatchHierarchicalCmd())
	matchCmd.AddCommand(createMatchRuleCmd())
	matchCmd.AddCommand(createMatchVectorCmd())
	matchCmd.AddCommand(createReviewCmd())
	matchCmd.AddCommand(createAnalyzeCmd())
	matchCmd.AddCommand(createExportCmd())

	return matchCmd
}

func createMatchDeterministicCmd() *cobra.Command {
	var runLabel string
	var batchSize int

	cmd := &cobra.Command{
		Use:   "deterministic",
		Short: "Run deterministic matching (Stage 1)",
		Long:  `Run Stage 1 deterministic matching: legacy UPRN validation and exact canonical address matches`,
		Run: func(cmd *cobra.Command, args []string) {
			if runLabel == "" {
				runLabel = fmt.Sprintf("deterministic-%d", time.Now().Unix())
			}

			// Create match engine and run
			matchEngine := engine.NewMatchEngine(dbConn.DB)
			
			// Create matching run
			run, err := matchEngine.CreateMatchRun(runLabel, "v1.0", "Deterministic matching: legacy UPRN validation + exact canonical matches")
			if err != nil {
				log.Fatalf("Failed to create match run: %v", err)
			}

			// Run deterministic matching
			deterministicMatcher := engine.NewDeterministicMatcher(dbConn.DB)
			totalProcessed, totalAccepted, err := deterministicMatcher.RunDeterministicMatching(run.RunID, batchSize)
			if err != nil {
				log.Fatalf("Deterministic matching failed: %v", err)
			}

			// Complete the run
			needsReview := 0 // TODO: Count from match_result table
			rejected := 0    // TODO: Count from match_result table
			
			err = matchEngine.CompleteMatchRun(run.RunID, totalProcessed, totalAccepted, needsReview, rejected)
			if err != nil {
				log.Printf("Failed to complete match run: %v", err)
			}

			fmt.Printf("\n=== Deterministic Matching Results ===\n")
			fmt.Printf("Run ID: %d\n", run.RunID)
			fmt.Printf("Run Label: %s\n", runLabel)
			fmt.Printf("Total Processed: %d\n", totalProcessed)
			fmt.Printf("Auto-Accepted: %d\n", totalAccepted)
			fmt.Printf("Coverage: %.2f%%\n", float64(totalAccepted)/float64(totalProcessed)*100)
		},
	}

	cmd.Flags().StringVar(&runLabel, "label", "", "Label for this matching run")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "Batch size for processing documents")

	return cmd
}

func createMatchFuzzyCmd() *cobra.Command {
	var runLabel string
	var batchSize int
	var minSimilarity float64

	cmd := &cobra.Command{
		Use:   "fuzzy",
		Short: "Run fuzzy matching (Stage 2)",
		Long:  `Run Stage 2 fuzzy matching using PostgreSQL trigram similarity with filtering`,
		Run: func(cmd *cobra.Command, args []string) {
			if runLabel == "" {
				runLabel = fmt.Sprintf("fuzzy-%d", time.Now().Unix())
			}

			// Create match engine and run
			matchEngine := engine.NewMatchEngine(dbConn.DB)
			
			// Create matching run
			run, err := matchEngine.CreateMatchRun(runLabel, "v1.0", fmt.Sprintf("Fuzzy matching: pg_trgm similarity >= %.2f with phonetic and structural filtering", minSimilarity))
			if err != nil {
				log.Fatalf("Failed to create match run: %v", err)
			}

			// Configure fuzzy matching tiers
			tiers := engine.DefaultTiers()
			tiers.MinThreshold = minSimilarity

			// Run fuzzy matching
			fuzzyMatcher := engine.NewFuzzyMatcher(dbConn.DB)
			totalProcessed, totalAccepted, totalNeedsReview, err := fuzzyMatcher.RunFuzzyMatching(run.RunID, batchSize, tiers)
			if err != nil {
				log.Fatalf("Fuzzy matching failed: %v", err)
			}

			// Complete the run
			rejected := totalProcessed - totalAccepted - totalNeedsReview
			
			err = matchEngine.CompleteMatchRun(run.RunID, totalProcessed, totalAccepted, totalNeedsReview, rejected)
			if err != nil {
				log.Printf("Failed to complete match run: %v", err)
			}

			fmt.Printf("\n=== Fuzzy Matching Results ===\n")
			fmt.Printf("Run ID: %d\n", run.RunID)
			fmt.Printf("Run Label: %s\n", runLabel)
			fmt.Printf("Total Processed: %d\n", totalProcessed)
			fmt.Printf("Auto-Accepted: %d\n", totalAccepted)
			fmt.Printf("Needs Review: %d\n", totalNeedsReview)
			fmt.Printf("Rejected: %d\n", rejected)
			fmt.Printf("Coverage: %.2f%%\n", float64(totalAccepted)/float64(totalProcessed)*100)
			fmt.Printf("Review Rate: %.2f%%\n", float64(totalNeedsReview)/float64(totalProcessed)*100)
		},
	}

	cmd.Flags().StringVar(&runLabel, "label", "", "Label for this matching run")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "Batch size for processing documents")
	cmd.Flags().Float64Var(&minSimilarity, "min-similarity", 0.60, "Minimum trigram similarity threshold (optimized default)")

	return cmd
}

func createMatchOptimizedCmd() *cobra.Command {
	var runLabel string
	var batchSize int
	var minSimilarity float64
	var workers int

	cmd := &cobra.Command{
		Use:   "fuzzy-optimized",
		Short: "Run optimized fuzzy matching with parallel processing",
		Long:  `Run optimized Stage 2 fuzzy matching with parallel workers and caching for better performance`,
		Run: func(cmd *cobra.Command, args []string) {
			if runLabel == "" {
				runLabel = fmt.Sprintf("fuzzy-opt-%d", time.Now().Unix())
			}

			// Create match engine and run
			matchEngine := engine.NewMatchEngine(dbConn.DB)
			
			// Create matching run
			run, err := matchEngine.CreateMatchRun(runLabel, "v2.0", 
				fmt.Sprintf("Optimized fuzzy matching: %d workers, similarity >= %.2f", workers, minSimilarity))
			if err != nil {
				log.Fatalf("Failed to create match run: %v", err)
			}

			// Configure fuzzy matching tiers
			tiers := engine.DefaultTiers()
			tiers.MinThreshold = minSimilarity

			// Run optimized fuzzy matching
			optimizedMatcher := engine.NewOptimizedFuzzyMatcher(dbConn.DB, workers)
			totalProcessed, totalAccepted, totalNeedsReview, err := optimizedMatcher.RunOptimizedFuzzyMatching(run.RunID, batchSize, tiers)
			if err != nil {
				log.Fatalf("Optimized fuzzy matching failed: %v", err)
			}

			// Complete the run
			rejected := totalProcessed - totalAccepted - totalNeedsReview
			
			err = matchEngine.CompleteMatchRun(run.RunID, totalProcessed, totalAccepted, totalNeedsReview, rejected)
			if err != nil {
				log.Printf("Failed to complete match run: %v", err)
			}

			fmt.Printf("\n=== Optimized Fuzzy Matching Results ===\n")
			fmt.Printf("Run ID: %d\n", run.RunID)
			fmt.Printf("Run Label: %s\n", runLabel)
			fmt.Printf("Workers: %d\n", workers)
			fmt.Printf("Total Processed: %d\n", totalProcessed)
			fmt.Printf("Auto-Accepted: %d\n", totalAccepted)
			fmt.Printf("Needs Review: %d\n", totalNeedsReview)
			fmt.Printf("Rejected: %d\n", rejected)
			fmt.Printf("Coverage: %.2f%%\n", float64(totalAccepted)/float64(totalProcessed)*100)
			fmt.Printf("Review Rate: %.2f%%\n", float64(totalNeedsReview)/float64(totalProcessed)*100)
		},
	}

	cmd.Flags().StringVar(&runLabel, "label", "", "Label for this matching run")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "Batch size for processing documents")
	cmd.Flags().Float64Var(&minSimilarity, "min-similarity", 0.60, "Minimum trigram similarity threshold (optimized default)")
	cmd.Flags().IntVar(&workers, "workers", 4, "Number of parallel workers")

	return cmd
}

func createTuneThresholdsCmd() *cobra.Command {
	var sampleSize int
	var analyze bool

	cmd := &cobra.Command{
		Use:   "tune-thresholds",
		Short: "Find optimal similarity thresholds",
		Long:  `Test different similarity thresholds to find optimal settings for precision and recall`,
		Run: func(cmd *cobra.Command, args []string) {
			tuner := engine.NewThresholdTuner(dbConn.DB)

			if analyze {
				// Just analyze current matches
				if err := tuner.AnalyzeCurrentMatches(); err != nil {
					log.Printf("Failed to analyze matches: %v", err)
				}
				return
			}

			// Run threshold tuning
			results, err := tuner.TestThresholds(sampleSize)
			if err != nil {
				log.Fatalf("Threshold tuning failed: %v", err)
			}

			// Print detailed results table
			fmt.Println("\n=== Detailed Threshold Analysis ===")
			fmt.Println("Threshold | Precision | Recall | F1 Score | Auto | Review | Time(s)")
			fmt.Println("----------|-----------|--------|----------|------|--------|--------")
			
			for _, r := range results {
				fmt.Printf("   %.2f   |  %.2f%%  | %.2f%% |  %.3f   | %4d |  %4d  | %.2f\n",
					r.Threshold, 
					r.Precision*100, 
					r.Recall*100, 
					r.F1Score,
					r.AutoAcceptCount,
					r.ReviewCount,
					r.ProcessingTime.Seconds())
			}
		},
	}

	cmd.Flags().IntVar(&sampleSize, "sample-size", 500, "Number of documents to test")
	cmd.Flags().BoolVar(&analyze, "analyze", false, "Analyze existing matches instead of tuning")

	return cmd
}

func createMatchPostcodeCmd() *cobra.Command {
	var runLabel string
	var batchSize int

	cmd := &cobra.Command{
		Use:   "postcode",
		Short: "Run postcode-centric matching",
		Long:  `Match addresses based on postcodes with component matching`,
		Run: func(cmd *cobra.Command, args []string) {
			if runLabel == "" {
				runLabel = fmt.Sprintf("postcode-%d", time.Now().Unix())
			}

			// Create match engine and run
			matchEngine := engine.NewMatchEngine(dbConn.DB)
			
			// Create matching run
			run, err := matchEngine.CreateMatchRun(runLabel, "v1.0", 
				"Postcode-centric matching with component analysis")
			if err != nil {
				log.Fatalf("Failed to create match run: %v", err)
			}

			// Run postcode matching
			postcodeMatcher := engine.NewPostcodeMatcher(dbConn.DB)
			totalProcessed, totalAccepted, totalNeedsReview, err := postcodeMatcher.RunPostcodeMatching(run.RunID, batchSize)
			if err != nil {
				log.Fatalf("Postcode matching failed: %v", err)
			}

			// Complete the run
			rejected := totalProcessed - totalAccepted - totalNeedsReview
			
			err = matchEngine.CompleteMatchRun(run.RunID, totalProcessed, totalAccepted, totalNeedsReview, rejected)
			if err != nil {
				log.Printf("Failed to complete match run: %v", err)
			}

			fmt.Printf("\n=== Postcode Matching Results ===\n")
			fmt.Printf("Run ID: %d\n", run.RunID)
			fmt.Printf("Run Label: %s\n", runLabel)
			fmt.Printf("Total Processed: %d\n", totalProcessed)
			fmt.Printf("Auto-Accepted: %d\n", totalAccepted)
			fmt.Printf("Needs Review: %d\n", totalNeedsReview)
			fmt.Printf("Rejected: %d\n", rejected)
			fmt.Printf("Coverage: %.2f%%\n", float64(totalAccepted)/float64(totalProcessed)*100)
			fmt.Printf("Review Rate: %.2f%%\n", float64(totalNeedsReview)/float64(totalProcessed)*100)
		},
	}

	cmd.Flags().StringVar(&runLabel, "label", "", "Label for this matching run")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "Batch size for processing documents")

	return cmd
}

func createAnalyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze data quality and matching potential",
		Long:  `Analyze source data quality and potential for different matching strategies`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("=== EHDC LLPG Data Analysis ===\n")

			// Overall statistics
			var totalDocs, matchedDocs, unmatchedDocs int
			err := dbConn.DB.QueryRow(`
				SELECT 
					COUNT(*) as total,
					COUNT(m.src_id) as matched,
					COUNT(*) - COUNT(m.src_id) as unmatched
				FROM src_document s
				LEFT JOIN match_accepted m ON m.src_id = s.src_id
			`).Scan(&totalDocs, &matchedDocs, &unmatchedDocs)
			
			if err == nil {
				fmt.Printf("Total Documents: %d\n", totalDocs)
				fmt.Printf("Matched Documents: %d (%.2f%%)\n", 
					matchedDocs, float64(matchedDocs)/float64(totalDocs)*100)
				fmt.Printf("Unmatched Documents: %d (%.2f%%)\n", 
					unmatchedDocs, float64(unmatchedDocs)/float64(totalDocs)*100)
			}

			// Analyze postcode quality
			postcodeMatcher := engine.NewPostcodeMatcher(dbConn.DB)
			if err := postcodeMatcher.AnalyzePostcodeQuality(); err != nil {
				log.Printf("Failed to analyze postcode quality: %v", err)
			}

			// Analyze address quality
			fmt.Println("\n=== Address Quality Analysis ===")
			
			rows, err := dbConn.DB.Query(`
				SELECT 
					source_type,
					COUNT(*) as total,
					COUNT(CASE WHEN addr_can IS NOT NULL AND addr_can != 'N A' THEN 1 END) as with_address,
					COUNT(CASE WHEN postcode_text IS NOT NULL AND postcode_text != '' THEN 1 END) as with_postcode,
					COUNT(CASE WHEN easting_raw IS NOT NULL THEN 1 END) as with_coords,
					COUNT(CASE WHEN uprn_raw IS NOT NULL AND uprn_raw != '' THEN 1 END) as with_uprn
				FROM src_document
				GROUP BY source_type
				ORDER BY total DESC
			`)
			
			if err == nil {
				defer rows.Close()
				fmt.Println("Type       | Total  | Address | Postcode | Coords | UPRN")
				fmt.Println("-----------|--------|---------|----------|--------|------")
				
				for rows.Next() {
					var sourceType string
					var total, withAddr, withPostcode, withCoords, withUPRN int
					
					err := rows.Scan(&sourceType, &total, &withAddr, &withPostcode, &withCoords, &withUPRN)
					if err == nil {
						fmt.Printf("%-10s | %6d | %6.1f%% | %7.1f%% | %5.1f%% | %4.1f%%\n",
							sourceType, total,
							float64(withAddr)/float64(total)*100,
							float64(withPostcode)/float64(total)*100,
							float64(withCoords)/float64(total)*100,
							float64(withUPRN)/float64(total)*100)
					}
				}
			}

			// Matching method effectiveness
			fmt.Println("\n=== Matching Method Effectiveness ===")
			
			rows2, err := dbConn.DB.Query(`
				SELECT 
					CASE 
						WHEN method LIKE 'deterministic%' THEN 'Deterministic'
						WHEN method LIKE 'fuzzy%' THEN 'Fuzzy'
						WHEN method LIKE 'postcode%' THEN 'Postcode'
						ELSE 'Other'
					END as method_type,
					COUNT(*) as matches,
					AVG(confidence) as avg_confidence
				FROM match_accepted
				GROUP BY method_type
				ORDER BY matches DESC
			`)
			
			if err == nil {
				defer rows2.Close()
				fmt.Println("Method        | Matches | Avg Confidence")
				fmt.Println("--------------|---------|---------------")
				
				for rows2.Next() {
					var methodType string
					var matches int
					var avgConf float64
					
					if err := rows2.Scan(&methodType, &matches, &avgConf); err == nil {
						fmt.Printf("%-13s | %7d | %.3f\n", methodType, matches, avgConf)
					}
				}
			}
		},
	}

	return cmd
}

func createMatchSpatialCmd() *cobra.Command {
	var runLabel string
	var batchSize int
	var maxDistance float64

	cmd := &cobra.Command{
		Use:   "spatial",
		Short: "Run spatial proximity matching",
		Long:  `Match addresses based on spatial proximity using coordinates`,
		Run: func(cmd *cobra.Command, args []string) {
			if runLabel == "" {
				runLabel = fmt.Sprintf("spatial-%d", time.Now().Unix())
			}

			matchEngine := engine.NewMatchEngine(dbConn.DB)
			run, err := matchEngine.CreateMatchRun(runLabel, "v1.0", 
				fmt.Sprintf("Spatial proximity matching (max distance: %.0fm)", maxDistance))
			if err != nil {
				log.Fatalf("Failed to create match run: %v", err)
			}

			spatialMatcher := engine.NewSpatialMatcher(dbConn.DB)
			totalProcessed, totalAccepted, totalNeedsReview, err := spatialMatcher.RunSpatialMatching(run.RunID, batchSize, maxDistance)
			if err != nil {
				log.Fatalf("Spatial matching failed: %v", err)
			}

			rejected := totalProcessed - totalAccepted - totalNeedsReview
			err = matchEngine.CompleteMatchRun(run.RunID, totalProcessed, totalAccepted, totalNeedsReview, rejected)
			if err != nil {
				log.Printf("Failed to complete match run: %v", err)
			}

			fmt.Printf("\n=== Spatial Matching Results ===\n")
			fmt.Printf("Run ID: %d\n", run.RunID)
			fmt.Printf("Total Processed: %d\n", totalProcessed)
			fmt.Printf("Auto-Accepted: %d\n", totalAccepted)
			fmt.Printf("Needs Review: %d\n", totalNeedsReview)
			fmt.Printf("Coverage: %.2f%%\n", float64(totalAccepted)/float64(totalProcessed)*100)
		},
	}

	cmd.Flags().StringVar(&runLabel, "label", "", "Label for this matching run")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "Batch size for processing documents")
	cmd.Flags().Float64Var(&maxDistance, "max-distance", 100.0, "Maximum distance in meters")

	return cmd
}

func createMatchHierarchicalCmd() *cobra.Command {
	var runLabel string
	var batchSize int

	cmd := &cobra.Command{
		Use:   "hierarchical",
		Short: "Run hierarchical component matching",
		Long:  `Match addresses using hierarchical component-based approach`,
		Run: func(cmd *cobra.Command, args []string) {
			if runLabel == "" {
				runLabel = fmt.Sprintf("hierarchical-%d", time.Now().Unix())
			}

			matchEngine := engine.NewMatchEngine(dbConn.DB)
			run, err := matchEngine.CreateMatchRun(runLabel, "v1.0", 
				"Hierarchical component matching with multi-level fallbacks")
			if err != nil {
				log.Fatalf("Failed to create match run: %v", err)
			}

			hierarchicalMatcher := engine.NewHierarchicalMatcher(dbConn.DB)
			totalProcessed, totalAccepted, totalNeedsReview, err := hierarchicalMatcher.RunHierarchicalMatching(run.RunID, batchSize)
			if err != nil {
				log.Fatalf("Hierarchical matching failed: %v", err)
			}

			rejected := totalProcessed - totalAccepted - totalNeedsReview
			err = matchEngine.CompleteMatchRun(run.RunID, totalProcessed, totalAccepted, totalNeedsReview, rejected)
			if err != nil {
				log.Printf("Failed to complete match run: %v", err)
			}

			fmt.Printf("\n=== Hierarchical Matching Results ===\n")
			fmt.Printf("Run ID: %d\n", run.RunID)
			fmt.Printf("Total Processed: %d\n", totalProcessed)
			fmt.Printf("Auto-Accepted: %d\n", totalAccepted)
			fmt.Printf("Needs Review: %d\n", totalNeedsReview)
			fmt.Printf("Coverage: %.2f%%\n", float64(totalAccepted)/float64(totalProcessed)*100)
		},
	}

	cmd.Flags().StringVar(&runLabel, "label", "", "Label for this matching run")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "Batch size for processing documents")

	return cmd
}

func createMatchRuleCmd() *cobra.Command {
	var runLabel string
	var batchSize int

	cmd := &cobra.Command{
		Use:   "rule-based",
		Short: "Run rule-based pattern matching",
		Long:  `Match addresses using predefined transformation rules for known patterns`,
		Run: func(cmd *cobra.Command, args []string) {
			if runLabel == "" {
				runLabel = fmt.Sprintf("rule-based-%d", time.Now().Unix())
			}

			matchEngine := engine.NewMatchEngine(dbConn.DB)
			run, err := matchEngine.CreateMatchRun(runLabel, "v1.0", 
				"Rule-based pattern matching with known address transformations")
			if err != nil {
				log.Fatalf("Failed to create match run: %v", err)
			}

			ruleMatcher := engine.NewRuleMatcher(dbConn.DB)
			totalProcessed, totalAccepted, totalNeedsReview, err := ruleMatcher.RunRuleMatching(run.RunID, batchSize)
			if err != nil {
				log.Fatalf("Rule-based matching failed: %v", err)
			}

			rejected := totalProcessed - totalAccepted - totalNeedsReview
			err = matchEngine.CompleteMatchRun(run.RunID, totalProcessed, totalAccepted, totalNeedsReview, rejected)
			if err != nil {
				log.Printf("Failed to complete match run: %v", err)
			}

			fmt.Printf("\n=== Rule-Based Matching Results ===\n")
			fmt.Printf("Run ID: %d\n", run.RunID)
			fmt.Printf("Total Processed: %d\n", totalProcessed)
			fmt.Printf("Auto-Accepted: %d\n", totalAccepted)
			fmt.Printf("Needs Review: %d\n", totalNeedsReview)
			fmt.Printf("Coverage: %.2f%%\n", float64(totalAccepted)/float64(totalProcessed)*100)
		},
	}

	cmd.Flags().StringVar(&runLabel, "label", "", "Label for this matching run")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "Batch size for processing documents")

	return cmd
}

func createMatchVectorCmd() *cobra.Command {
	var runLabel string
	var batchSize int
	var minSimilarity float64
	var embeddingAPI string

	cmd := &cobra.Command{
		Use:   "vector",
		Short: "Run vector/semantic matching",
		Long:  `Match addresses using semantic embeddings and vector similarity`,
		Run: func(cmd *cobra.Command, args []string) {
			if runLabel == "" {
				runLabel = fmt.Sprintf("vector-%d", time.Now().Unix())
			}

			matchEngine := engine.NewMatchEngine(dbConn.DB)
			run, err := matchEngine.CreateMatchRun(runLabel, "v1.0", 
				fmt.Sprintf("Vector/semantic matching (min similarity: %.2f)", minSimilarity))
			if err != nil {
				log.Fatalf("Failed to create match run: %v", err)
			}

			vectorMatcher := engine.NewVectorMatcher(dbConn.DB, embeddingAPI)
			totalProcessed, totalAccepted, totalNeedsReview, err := vectorMatcher.RunVectorMatching(run.RunID, batchSize, minSimilarity)
			if err != nil {
				log.Fatalf("Vector matching failed: %v", err)
			}

			rejected := totalProcessed - totalAccepted - totalNeedsReview
			err = matchEngine.CompleteMatchRun(run.RunID, totalProcessed, totalAccepted, totalNeedsReview, rejected)
			if err != nil {
				log.Printf("Failed to complete match run: %v", err)
			}

			fmt.Printf("\n=== Vector Matching Results ===\n")
			fmt.Printf("Run ID: %d\n", run.RunID)
			fmt.Printf("Total Processed: %d\n", totalProcessed)
			fmt.Printf("Auto-Accepted: %d\n", totalAccepted)
			fmt.Printf("Needs Review: %d\n", totalNeedsReview)
			fmt.Printf("Coverage: %.2f%%\n", float64(totalAccepted)/float64(totalProcessed)*100)
		},
	}

	cmd.Flags().StringVar(&runLabel, "label", "", "Label for this matching run")
	cmd.Flags().IntVar(&batchSize, "batch-size", 100, "Batch size for processing documents")
	cmd.Flags().Float64Var(&minSimilarity, "min-similarity", 0.70, "Minimum semantic similarity")
	cmd.Flags().StringVar(&embeddingAPI, "embedding-api", "", "Embedding API endpoint (optional)")

	return cmd
}

func createReviewCmd() *cobra.Command {
	var batchSize int
	var reviewer string
	var showStats bool

	cmd := &cobra.Command{
		Use:   "review",
		Short: "Interactive manual review interface",
		Long:  `Start interactive session to manually review candidate matches`,
		Run: func(cmd *cobra.Command, args []string) {
			reviewInterface := engine.NewReviewInterface(dbConn.DB)
			
			if showStats {
				if err := reviewInterface.GetReviewStats(); err != nil {
					log.Printf("Failed to get review stats: %v", err)
				}
				return
			}

			if err := reviewInterface.RunInteractiveReview(batchSize, reviewer); err != nil {
				log.Fatalf("Review session failed: %v", err)
			}
		},
	}

	cmd.Flags().IntVar(&batchSize, "batch-size", 10, "Number of items to review in each batch")
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "Reviewer name for audit trail")
	cmd.Flags().BoolVar(&showStats, "stats", false, "Show review queue statistics only")

	return cmd
}

func createExportCmd() *cobra.Command {
	var outputDir string
	var showStats bool
	
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export enhanced source documents with matching results",
		Long:  `Export all source documents as enhanced CSVs with additional matching columns including Address_Quality, Match_Status, Match_Method, Match_Score, Coordinate_Distance, and Address_Similarity`,
		Run: func(cmd *cobra.Command, args []string) {
			exporter := engine.NewExporter(dbConn.DB)
			
			if showStats {
				// Show export statistics
				stats, err := exporter.GetExportStats()
				if err != nil {
					log.Fatalf("Failed to get export stats: %v", err)
				}
				
				fmt.Println("=== Export Statistics ===\n")
				
				if sourceStats, ok := stats["by_source_type"].(map[string]map[string]int); ok {
					fmt.Println("By Source Type:")
					fmt.Println("Type        | Total  | Matched | Unmatched | Match Rate")
					fmt.Println("------------|--------|---------|-----------|----------")
					
					for sourceType, counts := range sourceStats {
						total := counts["total"]
						matched := counts["matched"]
						unmatched := counts["unmatched"]
						rate := float64(matched) / float64(total) * 100
						
						fmt.Printf("%-11s | %6d | %7d | %9d | %7.1f%%\n", 
							sourceType, total, matched, unmatched, rate)
					}
				}
				
				fmt.Printf("\nOverall Summary:\n")
				fmt.Printf("Total Documents: %v\n", stats["total_documents"])
				fmt.Printf("Total Matched: %v\n", stats["total_matched"])  
				fmt.Printf("Total Unmatched: %v\n", stats["total_unmatched"])
				fmt.Printf("Overall Match Rate: %.2f%%\n", stats["match_rate"])
				
				return
			}
			
			// Perform the export
			if err := exporter.ExportEnhancedCSVs(outputDir); err != nil {
				log.Fatalf("Export failed: %v", err)
			}
		},
	}
	
	cmd.Flags().StringVar(&outputDir, "output", "export", "Output directory for CSV files")
	cmd.Flags().BoolVar(&showStats, "stats", false, "Show export statistics only")
	
	return cmd
}

func createDBCmd() *cobra.Command {
	dbCmd := &cobra.Command{
		Use:   "db",
		Short: "Database utility commands",
		Long:  `Database utilities for schema updates and maintenance`,
	}
	
	dbCmd.AddCommand(createApplyViewsCmd())
	dbCmd.AddCommand(createTestViewsCmd())
	dbCmd.AddCommand(createInspectViewsCmd())
	
	return dbCmd
}

func createApplyViewsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply-views",
		Short: "Apply enhanced SQL views to database",
		Long:  `Create enhanced views for source documents with calculated quality metrics and matching results`,
		Run: func(cmd *cobra.Command, args []string) {
			dbUtil := engine.NewDBUtil(dbConn.DB)
			
			if err := dbUtil.ExecuteSQLFiles("sql/09_create_enhanced_views.sql", "sql/10_create_map_views_fixed.sql"); err != nil {
				log.Fatalf("Failed to apply views: %v", err)
			}
			
			fmt.Println("\n=== Testing Views ===")
			if err := dbUtil.TestViews(); err != nil {
				log.Printf("View testing failed: %v", err)
			}
			
			if err := dbUtil.ShowViewSamples(); err != nil {
				log.Printf("Failed to show samples: %v", err)
			}
		},
	}
}

func createTestViewsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test-views",
		Short: "Test enhanced database views",
		Long:  `Test that all enhanced views are working correctly and show sample data`,
		Run: func(cmd *cobra.Command, args []string) {
			dbUtil := engine.NewDBUtil(dbConn.DB)
			
			if err := dbUtil.TestViews(); err != nil {
				log.Fatalf("View testing failed: %v", err)
			}
			
			if err := dbUtil.ShowViewSamples(); err != nil {
				log.Fatalf("Failed to show samples: %v", err)
			}
		},
	}
}

func createInspectViewsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect-views",
		Short: "Inspect database views structure",
		Long:  `Inspect the structure and content of database views, particularly v_map_ views`,
		Run: func(cmd *cobra.Command, args []string) {
			inspector := engine.NewViewInspector(dbConn.DB)
			
			if err := inspector.InspectMapViews(); err != nil {
				log.Fatalf("View inspection failed: %v", err)
			}
		},
	}
}
