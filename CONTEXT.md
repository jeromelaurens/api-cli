# API CLI

Local API runner that lets agent skills call configured business APIs without exposing credentials to the model.

## Language

**API Service**:
An external business API system such as Dolibarr, Pennylane, Pipedrive, Crisp, MailerLite, or Sentry.
_Avoid_: service

**API Profile**:
A named local configuration for calling one **API Service**.
_Avoid_: service, account, connector

**Profile Catalog**:
The local non-secret collection of **API Profiles**.
_Avoid_: service config, connector config

**API Request**:
An HTTP request built from an **API Profile** and non-secret inputs supplied by an agent.
_Avoid_: raw request, direct request

**Credential**:
A confidential value stored outside model-visible configuration and used by an **API Profile**.
_Avoid_: token, secret, API key

**Auth Scheme**:
A non-secret rule that defines how an **API Profile** applies its **Credentials** to an **API Request**.
_Avoid_: auth type, credential type

**Mutation**:
An **API Request** using `POST`, `PUT`, `PATCH`, or `DELETE`.
_Avoid_: write request, unsafe request

**Response Envelope**:
A JSON object emitted by the CLI that describes the HTTP response or failure without exposing credentials.
_Avoid_: raw response, unwrapped response

**Redaction**:
The masking of known confidential values before CLI output is emitted.
_Avoid_: anonymization, sanitization

## Relationships

- An **API Service** can have one or more **API Profiles**.
- An **API Profile** belongs to exactly one **API Service**.
- A **Profile Catalog** contains zero or more **API Profiles**.
- An **API Request** targets exactly one **API Profile**.
- An **API Profile** has one **Auth Scheme** and one or more **Credentials**.
- A **Mutation** is a kind of **API Request**.
- A **Response Envelope** is emitted for every **API Request** result.
- **Redaction** applies to credentials and credential-derived values known to the CLI.

## Example Dialogue

> **Dev:** "Should the agent call the **API Service** directly?"
> **Domain expert:** "No, it calls an **API Profile**; the profile decides which external service and local credentials are used."

## Flagged Ambiguities

- "service" was used to mean both **API Service** and **API Profile**; resolved: CLI commands target an **API Profile**.
- "raw request" suggested unrestricted HTTP control; resolved: use **API Request** for profile-scoped HTTP calls built from non-secret agent inputs.
- "api-call" was the repository and prototype name; resolved: use **API CLI** for the product language.
- "raw JSON response" made success handling depend on upstream content type; resolved: use a **Response Envelope** for stable CLI output.
