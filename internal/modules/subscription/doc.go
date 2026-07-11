// Package subscription owns the subscription lifecycle state machine
// (trialing → active → past_due → grace → suspended → canceled|expired),
// period tracking, plan changes (immediate upgrade, scheduled downgrade),
// addon attach/detach, and renewal/trial jobs.
//
// It decides subscription policy and reacts to billing outcomes via events;
// it never calls a payment provider directly. Hexagonal layout. Implemented
// across T-019, T-020, and T-021.
package subscription
