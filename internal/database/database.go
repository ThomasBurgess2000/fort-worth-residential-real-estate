package database

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"acquisitions/internal/types"

	_ "github.com/sijms/go-ora/v2"
)

// dsn builds a properly encoded connection string for Oracle Autonomous Database
func dsn(username, password, host, port, service string, walletLocation string) string {
	if walletLocation != "" {
		// Use wallet-based mTLS connection
		return fmt.Sprintf(
			"oracle://%s:%s@%s:%s/%s?ssl=true&wallet_location=%s",
			username, password, host, port, service, url.PathEscape(walletLocation))
	}

	// Fallback to standard connection without wallet
	return (&url.URL{
		Scheme:   "oracle",
		User:     url.UserPassword(username, password), // escapes automatically
		Host:     host + ":" + port,
		Path:     "/" + service, // keep full service name
		RawQuery: "ssl=true",    // ADB requires TCPS on 1522
	}).String()
}

// loadEnvFile reads environment variables from a .env file
func loadEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err // File doesn't exist, which is okay
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		// Parse key=value format
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])

			// Remove quotes if present
			if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"') {
				value = value[1 : len(value)-1]
			}

			// Only set if not already set in environment
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}

	return scanner.Err()
}

// DBConfig holds database connection configuration
type DBConfig struct {
	Host           string
	Port           string
	Service        string
	Username       string
	Password       string
	WalletLocation string
}

// Database holds the database connection and configuration
type Database struct {
	db     *sql.DB
	config DBConfig
}

// NewDatabase creates a new database connection
func NewDatabase(config DBConfig) (*Database, error) {
	// Build properly encoded connection string for Oracle Autonomous Database
	connStr := dsn(config.Username, config.Password, config.Host, config.Port, config.Service, config.WalletLocation)

	// Debug: print connection string (without password)
	fmt.Printf("Connecting to Oracle Autonomous Database...\n")

	db, err := sql.Open("oracle", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Database{
		db:     db,
		config: config,
	}, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// QueryPropertyByAddress queries the database for a property by normalized address
func (d *Database) QueryPropertyByAddress(normalizedAddress string) (*types.Property, error) {
	query := `
		SELECT 
			Account_Num, Situs_Address, Owner_Name, Owner_Address, Owner_CityState, Owner_Zip,
			SubdivisionName, County, City, School, Land_Value, Improvement_Value, Total_Value,
			Deed_Date, ARB_Indicator, Year_Built, Living_Area, Num_Bedrooms, Num_Bathrooms,
			Property_Class, State_Use_Code, Land_Acres, Land_SqFt, Latitude, Longitude,
			Quality, LastSaleDate, Condition, DepreciationPercent, SiteClassCd, SiteClassDescr, LandUseCode
		FROM PROPERTYDATA_R_2025 
		WHERE UPPER(REPLACE(REPLACE(Situs_Address, ',', ''), '  ', ' ')) = :1
	`

	var prop types.Property
	err := d.db.QueryRow(query, normalizedAddress).Scan(
		&prop.AccountNum, &prop.SitusAddress, &prop.OwnerName, &prop.OwnerAddress, &prop.OwnerCityState, &prop.OwnerZip,
		&prop.Subdivision, &prop.County, &prop.City, &prop.SchoolDistrict, &prop.LandValue, &prop.ImprovementValue, &prop.TotalValue,
		&prop.DeedDate, &prop.ARBIndicator, &prop.YearBuilt, &prop.LivingArea, &prop.NumBedrooms, &prop.NumBathrooms,
		&prop.PropertyClass, &prop.StateUseCode, &prop.LandAcres, &prop.LandSqFt, &prop.Latitude, &prop.Longitude,
		&prop.Quality, &prop.LastSaleDate, &prop.Condition, &prop.DepreciationPercent, &prop.SiteClassCd, &prop.SiteClassDescr, &prop.LandUseCode,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Property not found
		}
		return nil, fmt.Errorf("failed to query property: %w", err)
	}

	return &prop, nil
}

// QueryPropertyByAddress2024 queries the 2024 data for a property by normalized address
func (d *Database) QueryPropertyByAddress2024(normalizedAddress string) (*types.Property, error) {
	query := `
		SELECT 
			Account_Num, Situs_Address, Owner_Name, Owner_Address, Owner_CityState, Owner_Zip,
			SubdivisionName, County, City, School, Land_Value, Improvement_Value, Total_Value,
			Deed_Date, ARB_Indicator, Year_Built, Living_Area, Num_Bedrooms, Num_Bathrooms,
			Property_Class, State_Use_Code, Land_Acres, Land_SqFt
		FROM PROPERTYDATA_2024 
		WHERE UPPER(REPLACE(REPLACE(Situs_Address, ',', ''), '  ', ' ')) = :1
	`

	var prop types.Property
	err := d.db.QueryRow(query, normalizedAddress).Scan(
		&prop.AccountNum, &prop.SitusAddress, &prop.OwnerName, &prop.OwnerAddress, &prop.OwnerCityState, &prop.OwnerZip,
		&prop.Subdivision, &prop.County, &prop.City, &prop.SchoolDistrict, &prop.LandValue, &prop.ImprovementValue, &prop.TotalValue,
		&prop.DeedDate, &prop.ARBIndicator, &prop.YearBuilt, &prop.LivingArea, &prop.NumBedrooms, &prop.NumBathrooms,
		&prop.PropertyClass, &prop.StateUseCode, &prop.LandAcres, &prop.LandSqFt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Property not found
		}
		return nil, fmt.Errorf("failed to query 2024 property: %w", err)
	}

	return &prop, nil
}

// QuerySubdivisionProperties queries all properties in a subdivision
func (d *Database) QuerySubdivisionProperties(subdivision string) ([]types.Property, error) {
	query := `
		SELECT 
			Account_Num, Situs_Address, Owner_Name, Owner_Address, Owner_CityState, Owner_Zip,
			SubdivisionName, County, City, School, Land_Value, Improvement_Value, Total_Value,
			Deed_Date, ARB_Indicator, Year_Built, Living_Area, Num_Bedrooms, Num_Bathrooms,
			Property_Class, State_Use_Code, Land_Acres, Land_SqFt, Latitude, Longitude,
			Quality, LastSaleDate, Condition, DepreciationPercent, SiteClassCd, SiteClassDescr, LandUseCode
		FROM PROPERTYDATA_R_2025 
		WHERE UPPER(SubdivisionName) = UPPER(:1)
	`

	rows, err := d.db.Query(query, subdivision)
	if err != nil {
		return nil, fmt.Errorf("failed to query subdivision properties: %w", err)
	}
	defer rows.Close()

	var properties []types.Property
	for rows.Next() {
		var prop types.Property
		err := rows.Scan(
			&prop.AccountNum, &prop.SitusAddress, &prop.OwnerName, &prop.OwnerAddress, &prop.OwnerCityState, &prop.OwnerZip,
			&prop.Subdivision, &prop.County, &prop.City, &prop.SchoolDistrict, &prop.LandValue, &prop.ImprovementValue, &prop.TotalValue,
			&prop.DeedDate, &prop.ARBIndicator, &prop.YearBuilt, &prop.LivingArea, &prop.NumBedrooms, &prop.NumBathrooms,
			&prop.PropertyClass, &prop.StateUseCode, &prop.LandAcres, &prop.LandSqFt, &prop.Latitude, &prop.Longitude,
			&prop.Quality, &prop.LastSaleDate, &prop.Condition, &prop.DepreciationPercent, &prop.SiteClassCd, &prop.SiteClassDescr, &prop.LandUseCode,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan property: %w", err)
		}
		properties = append(properties, prop)
	}

	return properties, nil
}

// QueryLargeLandProperties queries properties with large land areas (>10 acres)
func (d *Database) QueryLargeLandProperties() ([]types.Property, error) {
	query := `
		SELECT 
			Account_Num, Situs_Address, Owner_Name, Owner_Address, Owner_CityState, Owner_Zip,
			SubdivisionName, County, City, School, Land_Value, Improvement_Value, Total_Value,
			Deed_Date, ARB_Indicator, Year_Built, Living_Area, Num_Bedrooms, Num_Bathrooms,
			Property_Class, State_Use_Code, Land_Acres, Land_SqFt, Latitude, Longitude,
			Quality, LastSaleDate, Condition, DepreciationPercent, SiteClassCd, SiteClassDescr, LandUseCode
		FROM PROPERTYDATA_R_2025 
		WHERE TO_NUMBER(Land_Acres) > 10
		ORDER BY TO_NUMBER(Land_Acres) DESC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query large land properties: %w", err)
	}
	defer rows.Close()

	var properties []types.Property
	for rows.Next() {
		var prop types.Property
		err := rows.Scan(
			&prop.AccountNum, &prop.SitusAddress, &prop.OwnerName, &prop.OwnerAddress, &prop.OwnerCityState, &prop.OwnerZip,
			&prop.Subdivision, &prop.County, &prop.City, &prop.SchoolDistrict, &prop.LandValue, &prop.ImprovementValue, &prop.TotalValue,
			&prop.DeedDate, &prop.ARBIndicator, &prop.YearBuilt, &prop.LivingArea, &prop.NumBedrooms, &prop.NumBathrooms,
			&prop.PropertyClass, &prop.StateUseCode, &prop.LandAcres, &prop.LandSqFt, &prop.Latitude, &prop.Longitude,
			&prop.Quality, &prop.LastSaleDate, &prop.Condition, &prop.DepreciationPercent, &prop.SiteClassCd, &prop.SiteClassDescr, &prop.LandUseCode,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan property: %w", err)
		}
		properties = append(properties, prop)
	}

	return properties, nil
}

// LoadDatabaseConfig loads database configuration from environment variables
func LoadDatabaseConfig() DBConfig {
	// Try to load from .env file first
	loadEnvFile(".env")

	return DBConfig{
		Host:           getEnvOrDefault("DB_HOST", "localhost"),
		Port:           getEnvOrDefault("DB_PORT", "1521"),
		Service:        getEnvOrDefault("DB_SERVICE", "XE"),
		Username:       getEnvOrDefault("DB_USERNAME", ""),
		Password:       getEnvOrDefault("DB_PASSWORD", ""),
		WalletLocation: getEnvOrDefault("DB_WALLET_LOCATION", ""),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
