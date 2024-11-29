package database

import (
	"github.com/gocql/gocql"
	"log"
)

func InitSchema(session *gocql.Session) error {
	// Create keyspace
	if err := session.Query(`
		CREATE KEYSPACE IF NOT EXISTS triggerx
		WITH replication = {
			'class': 'SimpleStrategy',
			'replication_factor': 1
		}`).Exec(); err != nil {
		return err
	}

	// Drop existing tables if any

	// Create User_data table
	if err := session.Query(`
		CREATE TABLE IF NOT EXISTS triggerx.user_data (
			user_id bigint PRIMARY KEY,
			user_address text CHECK (user_address MATCHES '^0x[0-9a-fA-F]{40}$'),
			job_ids set<bigint>
		)`).Exec(); err != nil {
		return err
	}

	// Create Job_data table
	if err := session.Query(`
		CREATE TABLE IF NOT EXISTS triggerx.job_data (
			job_id bigint PRIMARY KEY,
			jobType int,
			user_id bigint,
			chain_id int,
			time_frame timestamp,
			time_interval int,
			contract_address text,
			target_function text,
			arg_type int,
			arguments list<text>,
			status boolean,
			job_cost_prediction decimal
		)`).Exec(); err != nil {
		return err
	}

	// Create Task_data table
	if err := session.Query(`
		CREATE TABLE IF NOT EXISTS triggerx.task_data (
			task_id bigint,
			job_id bigint,
			task_no int,
			quorum_id bigint,
			quorum_number int,
			quorum_threshold decimal,
			task_created_block bigint,
			task_created_tx_hash text,
			task_responded_block bigint,
			task_responded_tx_hash text,
			task_hash text,
			task_response_hash text,
			quorum_keeper_hash text,
			PRIMARY KEY (task_id)
		)`).Exec(); err != nil {
		return err
	}

	// Create Quorum_data table
	if err := session.Query(`
		CREATE TABLE IF NOT EXISTS triggerx.quorum_data (
			quorum_id bigint PRIMARY KEY,
			quorum_no int,
			quorum_creation_block bigint,
			quorum_tx_hash text,
			keepers list<text>,
			quorum_stake_total bigint,
			quorum_threshold decimal,
			task_ids set<bigint>
		)`).Exec(); err != nil {
		return err
	}

	// Create Keeper_data table
	if err := session.Query(`
		CREATE TABLE IF NOT EXISTS triggerx.keeper_data (
			keeper_id bigint PRIMARY KEY,
			withdrawal_address text CHECK (withdrawal_address MATCHES '^0x[0-9a-fA-F]{40}$'),
			stakes bigint,
			strategies int,
			verified boolean,
			status boolean,
			current_quorum_no int,
			registered_block_no bigint,
			register_tx_hash text,
			connection_address text,
			keystore_data text
		)`).Exec(); err != nil {
		return err
	}

	// Create Task_history table
	if err := session.Query(`
		CREATE TABLE IF NOT EXISTS triggerx.task_history (
			task_id bigint PRIMARY KEY,
			quorum_id bigint,
			keepers list<text>,
			responses list<text>,
			consensus_method text,
			validation_status boolean,
			tx_hash text
		)`).Exec(); err != nil {
		return err
	}

	log.Println("Database schema initialized successfully")
	return nil
} 