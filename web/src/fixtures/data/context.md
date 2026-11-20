# Context: Prepare the SSO login rollout plan for the app

> Purpose: **SSO rollout brain**: knowledge the team needs to plan and run the SSO login rollout.  
> 2026-11-20 09:30 · step 4 · ~797 tok · depth 2/2 · threshold 0.16 · hop 0.85  
> lit 23/38 memories + 8 concepts; the rest of memory is faded

## Focus

- **d121** · decision · w 0.76: We roll out SSO via OIDC with Entra ID
- **d123** · decision · w 0.76: SSO rollout deadline: 30 November
- **n150** · note · w 0.28: PM: client asks whether SSO will be ready by month end

## Concepts

◆ Identity / IdP 0.63 · ◆ Deadlines 0.60 · ◆ Client 0.57 · ◆ Authentication 0.44 · ◆ API integrations 0.40 · ◆ MFA 0.26 · ◆ Security 0.23 · ◆ Sensitive data 0.18

## Facts

- **f103** · fact · act 0.43 · d1 ← d121 · w 0.38  
  Entra ID supports OIDC and SAML 2.0
- **f102** · fact · act 0.38 · d1 ← d121 · w 0.31  
  The company uses Microsoft Entra ID
- **f104** · fact · act 0.24 · d2 ← A:client · w 0.42  
  The app has about 5000 active users
- **f101** · fact · act 0.20 · d2 ← A:auth · w 0.60  
  The app currently has e-mail + password login
- **f107** · fact · act 0.18 · d2 ← A:api · w 0.56  
  Graph API limit: 10k requests / 10 min per app
- **f110** · fact · act 0.17 · d2 ← A:auth · w 0.52  
  Audit: no rate limit on /login

## Tasks

- **t131** · task · act 0.46 · d1 ← d121 · w 0.50  
  Register the app in Entra (redirect URI, scopes)
- **t134** · task · act 0.41 · d1 ← d121 · w 0.52  
  E2E tests of the OIDC flow (happy path + consent denied)
- **t136** · task · act 0.40 · d1 ← d121 · w 0.50  
  Account mapping: e-mail → Entra oid

## Questions

- **q141** · question · act 0.22 · d2 ← A:idp · w 0.37  
  What about users outside the tenant (B2B guests)?

## Notes

- **n154** · note · act 0.17 · d2 ← A:client · w 0.14  
  Call Kim Aalto (+47 22 33 44 55, kim.aalto@halvorsen.example) before 15 Dec 2026 about the €48,000 renewal of Halvorsen Freight AS: 12% discount, see https://halvorsen.example/pricing, ticket #4821, IBAN NO93 8601 1117 947

## Files

- **x190** · file · act 0.38 · d1 ← d123 · w 0.45  
  rollout.csv — 3 rows (wave, users, owner, start_date)
- **x180** · file · act 0.35 · d1 ← d121 · w 0.30  
  oidc-flow.md — OIDC sequence and token handling
- **c182** · chunk · act 0.22 · d2 ← A:idp · w 0.32  
  The callback exchanges the code for tokens; the id_token oid claim maps to the local account.
- **c181** · chunk · act 0.16 · d2 ← A:auth · w 0.32  
  The browser is redirected to the Entra authorize endpoint with PKCE, state and nonce.

## Rollout

- **r193** · record · act 0.27 · d2 ← A:client · w 0.48  
  wave: everyone · users: 5000 · owner: Kim Lee · start_date: 2026-11-30
- **r191** · record · act 0.27 · d2 ← A:client · w 0.48  
  wave: pilot · users: 120 · owner: Kim Lee · start_date: 2026-11-24
- **r192** · record · act 0.23 · d2 ← A:client · w 0.48  
  wave: staff · users: 1400 · owner: Ana Ruiz · start_date: 2026-11-27

## Risks

- **k175** · risk · act 0.28 · d2 ← A:idp · w 0.49  
  Guest accounts may bypass conditional access

## Statistics

| measure | value |
|---|---|
| lit memories | 23 / 38 |
| active concepts | 8 |
| tokens (est.) | ~797 |

### Outdated versions (superseded, do not use)

- ~~d120: We plan SSO via SAML~~ → **d121**
- ~~d122: Deadline: end of October~~ → **d123**
