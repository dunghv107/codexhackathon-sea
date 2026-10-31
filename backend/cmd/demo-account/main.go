// Link a new local login to explicitly selected, previously unclaimed Olist records.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
)

func main() {
	username := flag.String("username", "", "New login name")
	buyerID := flag.String("buyer", "", "Optional existing buyer ID")
	shopIDs := flag.String("shops", "", "Comma-separated existing shop IDs")
	flag.Parse()
	password := os.Getenv("DEMO_PASSWORD")
	if err := validate(*username, password, *buyerID, *shopIDs); err != nil {
		log.Fatal(err)
	}
	if os.Getenv("DATABASE_URL") == "" {
		log.Fatal("DATABASE_URL is required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal("Cannot hash password")
	}
	db, err := gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatal("Cannot connect to database; check DATABASE_URL")
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL search_path TO sfd, public").Error; err != nil {
			return err
		}
		user := entity.User{Username: *username, Email: *username + "@demo.invalid", Password: string(hash), Role: entity.UserRoleUser, Status: entity.UserStatusActive}
		if err := tx.Table("public.users").Create(&user).Error; err != nil {
			return errors.New("Cannot create login (username/email may already exist); existing accounts are never overwritten")
		}
		subject := fmt.Sprintf("local:%d", user.ID)
		if *buyerID != "" {
			res := tx.Exec("UPDATE buyers SET auth_subject=? WHERE buyer_id=? AND auth_subject IS NULL", subject, *buyerID)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return errors.New("Buyer missing or already linked")
			}
		}
		if *shopIDs != "" {
			now := time.Now()
			seller := entity.Seller{SellerID: "live:" + uuid.NewString(), AuthSubject: &subject, DisplayName: *username, Status: entity.AccountStatusActive, AccountCreatedAt: &now, BatchID: entity.DemoBatchID, SourceRecordID: subject}
			if err := tx.Create(&seller).Error; err != nil {
				return err
			}
			for _, id := range strings.Split(*shopIDs, ",") {
				res := tx.Exec("UPDATE shops SET owner_seller_id=? WHERE shop_id=? AND owner_seller_id IS NULL", seller.SellerID, strings.TrimSpace(id))
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected != 1 {
					return fmt.Errorf("Shop %q missing, duplicated, or already linked", id)
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created login %s; selected profiles linked.\n", *username)
}

func validate(username, password, buyer, shops string) error {
	if len(username) < 3 || len(username) > 50 || strings.ContainsAny(username, " @\t\r\n") {
		return errors.New("Username must be 3–50 characters without whitespace or @")
	}
	if len(password) < 8 || len(password) > 72 {
		return errors.New("DEMO_PASSWORD must contain 8–72 bytes")
	}
	if buyer == "" && shops == "" {
		return errors.New("Select at least one buyer or shop")
	}
	if shops != "" {
		for _, id := range strings.Split(shops, ",") {
			if strings.TrimSpace(id) == "" {
				return errors.New("Shop IDs must not be empty")
			}
		}
	}
	return nil
}
