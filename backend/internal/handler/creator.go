package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// GetCreator handles GET /creators/{id} (admin-only).
//
// The authorisation check runs first so non-admin callers see 403 before any
// DB read; that keeps the response timing identical regardless of whether the
// creator exists. ErrCreatorNotFound from the service is mapped to 404
// CREATOR_NOT_FOUND by respondError. Like GetCreatorApplication, this handler
// deliberately logs nothing about the response body — every persisted field is
// PII-bearing (iin, names, phone, address, handles, telegram metadata) and
// must not surface in stdout-логах приложения per docs/standards/security.md.
func (s *Server) GetCreator(ctx context.Context, request api.GetCreatorRequestObject) (api.GetCreatorResponseObject, error) {
	if err := s.authzService.CanViewCreator(ctx); err != nil {
		return nil, err
	}

	aggregate, err := s.creatorService.GetByID(ctx, request.Id.String())
	if err != nil {
		return nil, err
	}

	data, err := domainCreatorAggregateToAPI(request.Id, aggregate)
	if err != nil {
		return nil, err
	}
	return api.GetCreator200JSONResponse{Data: data}, nil
}

// domainCreatorAggregateToAPI maps the service aggregate onto its strict-server
// counterpart. UUID parse failures on stored IDs (creator id is supplied via
// path and re-used as-is, but sourceApplicationId, social ids and
// verifiedByUserId come from DB columns) surface as wrapped errors so the
// strict-server adapter renders 500 — the rows are populated by trusted
// services so a parse failure means real corruption, not user input.
func domainCreatorAggregateToAPI(id openapi_types.UUID, a *domain.CreatorAggregate) (api.CreatorAggregate, error) {
	sourceApplicationID, err := uuid.Parse(a.SourceApplicationID)
	if err != nil {
		return api.CreatorAggregate{}, fmt.Errorf("parse source_application_id %q: %w", a.SourceApplicationID, err)
	}

	socials := make([]api.CreatorAggregateSocial, len(a.Socials))
	for i, social := range a.Socials {
		mapped, err := domainCreatorAggregateSocialToAPI(social)
		if err != nil {
			return api.CreatorAggregate{}, err
		}
		socials[i] = mapped
	}

	categories := make([]api.CreatorAggregateCategory, len(a.Categories))
	for i, c := range a.Categories {
		categories[i] = api.CreatorAggregateCategory{Code: c.Code, Name: c.Name}
	}

	return api.CreatorAggregate{
		Id:                  id,
		Iin:                 a.IIN,
		SourceApplicationId: sourceApplicationID,
		LastName:            a.LastName,
		FirstName:           a.FirstName,
		MiddleName:          a.MiddleName,
		BirthDate:           openapi_types.Date{Time: a.BirthDate},
		Phone:               a.Phone,
		CityCode:            a.CityCode,
		CityName:            a.CityName,
		Address:             a.Address,
		CategoryOtherText:   a.CategoryOtherText,
		TelegramUserId:      a.TelegramUserID,
		TelegramUsername:    a.TelegramUsername,
		TelegramFirstName:   a.TelegramFirstName,
		TelegramLastName:    a.TelegramLastName,
		Socials:             socials,
		Categories:          categories,
		CreatedAt:           a.CreatedAt,
		UpdatedAt:           a.UpdatedAt,
	}, nil
}

func domainCreatorAggregateSocialToAPI(s domain.CreatorAggregateSocial) (api.CreatorAggregateSocial, error) {
	socialID, err := uuid.Parse(s.ID)
	if err != nil {
		return api.CreatorAggregateSocial{}, fmt.Errorf("parse social id %q: %w", s.ID, err)
	}
	out := api.CreatorAggregateSocial{
		Id:        socialID,
		Platform:  api.SocialPlatform(s.Platform),
		Handle:    s.Handle,
		Verified:  s.Verified,
		CreatedAt: s.CreatedAt,
	}
	if s.Method != nil {
		m := api.SocialVerificationMethod(*s.Method)
		out.Method = &m
	}
	if s.VerifiedByUserID != nil {
		u, err := uuid.Parse(*s.VerifiedByUserID)
		if err != nil {
			return api.CreatorAggregateSocial{}, fmt.Errorf("parse verified_by_user_id %q: %w", *s.VerifiedByUserID, err)
		}
		out.VerifiedByUserId = &u
	}
	if s.VerifiedAt != nil {
		out.VerifiedAt = s.VerifiedAt
	}
	return out, nil
}
