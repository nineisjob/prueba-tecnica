// Package postgres implements domain.AuctionRepository, domain.BidRepository,
// and domain.UserRepository against a pgx connection pool. It is the only
// package that imports pgx directly (DIP: everything else depends on the
// domain interfaces).
//
// Each domain repository interface is backed by its own small struct
// (AuctionRepo, BidRepo, UserRepo) rather than one type implementing all
// three: Go has no method overloading, and AuctionRepository.Create(*Auction)
// and UserRepository.Create(*User) cannot both be satisfied by identically
// named methods on the same receiver type.
package postgres

import "github.com/jackc/pgx/v5/pgxpool"

type AuctionRepo struct{ pool *pgxpool.Pool }

func NewAuctionRepo(pool *pgxpool.Pool) *AuctionRepo { return &AuctionRepo{pool: pool} }

type BidRepo struct{ pool *pgxpool.Pool }

func NewBidRepo(pool *pgxpool.Pool) *BidRepo { return &BidRepo{pool: pool} }

type UserRepo struct{ pool *pgxpool.Pool }

func NewUserRepo(pool *pgxpool.Pool) *UserRepo { return &UserRepo{pool: pool} }
