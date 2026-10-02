-- Module: exchange. Guoke coin redemption; never creates or issues in-game assets.
-- +goose Up
CREATE TABLE exchange_shop_settings(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),isk_per_coin bigint NOT NULL DEFAULT 0 CHECK(isk_per_coin BETWEEN 0 AND 1000000000000),version bigint NOT NULL DEFAULT 1);
INSERT INTO exchange_shop_settings(singleton) VALUES(true);
CREATE TABLE exchange_rewards(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 type_id bigint NOT NULL CHECK(type_id>0),quantity integer NOT NULL CHECK(quantity BETWEEN 1 AND 1000000),
 isk_value bigint NOT NULL CHECK(isk_value BETWEEN 1 AND 1000000000000),stock integer NOT NULL CHECK(stock>=0),
 enabled boolean NOT NULL DEFAULT false,version bigint NOT NULL DEFAULT 1
);
CREATE TABLE exchange_redemptions(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,account_id uuid NOT NULL,request_key uuid NOT NULL,fingerprint text NOT NULL,
 reward_id bigint NOT NULL REFERENCES exchange_rewards(id),type_id bigint NOT NULL,quantity integer NOT NULL,
 recipient_id bigint NOT NULL,recipient_name text NOT NULL,isk_per_coin bigint NOT NULL CHECK(isk_per_coin>0),isk_value bigint NOT NULL CHECK(isk_value>0),coins_minor bigint NOT NULL CHECK(coins_minor>0),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','fulfilled','cancelled')),
 version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),decided_at timestamptz,decided_by uuid,note text NOT NULL DEFAULT '',
 UNIQUE(account_id,request_key)
);
CREATE INDEX exchange_redemption_account ON exchange_redemptions(account_id,id DESC);
CREATE TABLE exchange_shop_audit(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,actor_id uuid NOT NULL,request_key uuid NOT NULL,kind text NOT NULL,target_id bigint NOT NULL DEFAULT 0,fingerprint text NOT NULL,payload jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(actor_id,request_key));
CREATE TABLE exchange_source_rates(source_id text PRIMARY KEY,minor_per_unit bigint NOT NULL DEFAULT 0 CHECK(minor_per_unit BETWEEN 0 AND 100000000),version bigint NOT NULL DEFAULT 1);
INSERT INTO exchange_source_rates(source_id) VALUES('pap');
CREATE TABLE exchange_source_awards(source_id text NOT NULL,reference text NOT NULL,account_id uuid NOT NULL,units bigint NOT NULL,minor_per_unit bigint NOT NULL,coins bigint NOT NULL,PRIMARY KEY(source_id,reference));
CREATE TABLE exchange_coin_ledger(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,account_id uuid NOT NULL,kind text NOT NULL CHECK(kind IN('source','reserve','refund')),reference text NOT NULL,request_key uuid NOT NULL,delta bigint NOT NULL CHECK(delta<>0),reason text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(kind,reference,request_key));
CREATE INDEX exchange_coin_ledger_account ON exchange_coin_ledger(account_id,id DESC);
-- +goose Down
DROP TABLE exchange_coin_ledger;
DROP TABLE exchange_source_awards;
DROP TABLE exchange_source_rates;
DROP TABLE exchange_shop_audit;
DROP TABLE exchange_redemptions;
DROP TABLE exchange_rewards;
DROP TABLE exchange_shop_settings;
