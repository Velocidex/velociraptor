package users

// This module provides high level user management functions that
// consider the principal's ACLs in performing the various actions.

// The high level goal is to allow a SERVER_ADMIN as much power over
// users in their own org as possible, but not being able to interfere
// with other orgs.

// This module essentially sets the rules of interaction between orgs.
