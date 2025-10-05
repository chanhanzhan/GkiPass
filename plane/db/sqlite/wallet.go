package sqlite

import (
	"database/sql"
	"time"

	dbinit "gkipass/plane/db/init"

	"github.com/google/uuid"
)

// CreateWallet 创建钱包
func (s *SQLiteDB) CreateWallet(wallet *dbinit.Wallet) error {
	query := `
		INSERT INTO wallets (id, user_id, balance, frozen, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		wallet.ID,
		wallet.UserID,
		wallet.Balance,
		wallet.Frozen,
		wallet.CreatedAt,
		wallet.UpdatedAt,
	)
	return err
}

// GetWallet 获取钱包
func (s *SQLiteDB) GetWallet(id string) (*dbinit.Wallet, error) {
	wallet := &dbinit.Wallet{}
	query := `SELECT id, user_id, balance, frozen, created_at, updated_at FROM wallets WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&wallet.ID,
		&wallet.UserID,
		&wallet.Balance,
		&wallet.Frozen,
		&wallet.CreatedAt,
		&wallet.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return wallet, err
}

// GetWalletByUserID 根据用户ID获取钱包
func (s *SQLiteDB) GetWalletByUserID(userID string) (*dbinit.Wallet, error) {
	wallet := &dbinit.Wallet{}
	query := `SELECT id, user_id, balance, frozen, created_at, updated_at FROM wallets WHERE user_id = ?`
	err := s.db.QueryRow(query, userID).Scan(
		&wallet.ID,
		&wallet.UserID,
		&wallet.Balance,
		&wallet.Frozen,
		&wallet.CreatedAt,
		&wallet.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return wallet, err
}

// UpdateWalletBalance 更新钱包余额
func (s *SQLiteDB) UpdateWalletBalance(id string, balance, frozen float64) error {
	query := `UPDATE wallets SET balance = ?, frozen = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.Exec(query, balance, frozen, time.Now(), id)
	return err
}

// CreateWalletTransaction 创建交易记录
func (s *SQLiteDB) CreateWalletTransaction(tx *dbinit.WalletTransaction) error {
	if tx.ID == "" {
		tx.ID = uuid.New().String()
	}
	if tx.CreatedAt.IsZero() {
		tx.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO wallet_transactions 
		(id, wallet_id, user_id, type, amount, balance, related_id, related_type, 
		 status, payment_method, transaction_no, description, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		tx.ID,
		tx.WalletID,
		tx.UserID,
		tx.Type,
		tx.Amount,
		tx.Balance,
		tx.RelatedID,
		tx.RelatedType,
		tx.Status,
		tx.PaymentMethod,
		tx.TransactionNo,
		tx.Description,
		tx.CreatedAt,
	)
	return err
}

// GetWalletTransaction 获取交易记录
func (s *SQLiteDB) GetWalletTransaction(id string) (*dbinit.WalletTransaction, error) {
	tx := &dbinit.WalletTransaction{}
	query := `
		SELECT id, wallet_id, user_id, type, amount, balance, related_id, related_type,
		       status, payment_method, transaction_no, description, created_at
		FROM wallet_transactions WHERE id = ?
	`
	err := s.db.QueryRow(query, id).Scan(
		&tx.ID,
		&tx.WalletID,
		&tx.UserID,
		&tx.Type,
		&tx.Amount,
		&tx.Balance,
		&tx.RelatedID,
		&tx.RelatedType,
		&tx.Status,
		&tx.PaymentMethod,
		&tx.TransactionNo,
		&tx.Description,
		&tx.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return tx, err
}

// ListWalletTransactions 获取用户的交易记录列表
func (s *SQLiteDB) ListWalletTransactions(userID string, page, limit int) ([]*dbinit.WalletTransaction, int, error) {
	offset := (page - 1) * limit

	// 获取总数
	var total int
	countQuery := `SELECT COUNT(*) FROM wallet_transactions WHERE user_id = ?`
	err := s.db.QueryRow(countQuery, userID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 获取列表
	transactions := []*dbinit.WalletTransaction{}
	query := `
		SELECT id, wallet_id, user_id, type, amount, balance, related_id, related_type,
		       status, payment_method, transaction_no, description, created_at
		FROM wallet_transactions 
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := s.db.Query(query, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		tx := &dbinit.WalletTransaction{}
		err := rows.Scan(
			&tx.ID,
			&tx.WalletID,
			&tx.UserID,
			&tx.Type,
			&tx.Amount,
			&tx.Balance,
			&tx.RelatedID,
			&tx.RelatedType,
			&tx.Status,
			&tx.PaymentMethod,
			&tx.TransactionNo,
			&tx.Description,
			&tx.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		transactions = append(transactions, tx)
	}

	return transactions, total, rows.Err()
}

// UpdateTransactionStatus 更新交易状态
func (s *SQLiteDB) UpdateTransactionStatus(id, status string) error {
	query := `UPDATE wallet_transactions SET status = ? WHERE id = ?`
	_, err := s.db.Exec(query, status, id)
	return err
}

// GetOrCreateWallet 获取或创建用户钱包
func (s *SQLiteDB) GetOrCreateWallet(userID string) (*dbinit.Wallet, error) {
	wallet, err := s.GetWalletByUserID(userID)
	if err != nil {
		return nil, err
	}

	if wallet == nil {
		// 创建新钱包
		wallet = &dbinit.Wallet{
			ID:        uuid.New().String(),
			UserID:    userID,
			Balance:   0,
			Frozen:    0,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := s.CreateWallet(wallet); err != nil {
			return nil, err
		}
	}

	return wallet, nil
}
