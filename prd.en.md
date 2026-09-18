> 🌐 **English** | [中文](prd.md)

# AI Gateway and Model Guardrails System

## Product Requirements Document (PRD)


## Document Version

| Version | Date | Author | Change Description |
|------|------|------|----------|
| v1.0 | 2026-09-02 | - | Initial version |
| v1.1 | 2026-09-02 | - | Added optional MySQL database support; refined configuration hot-reload mechanism |


## Table of Contents

1. [Project Overview](#1-project-overview)
2. [User Roles and Use Cases](#2-user-roles-and-use-cases)
3. [Functional Requirements](#3-functional-requirements)
4. [UI Layout Planning](#4-ui-layout-planning)
5. [Non-Functional Requirements](#5-non-functional-requirements)
6. [Technical Implementation](#6-technical-implementation)
7. [Deployment Plan](#7-deployment-plan)
8. [Risks and Mitigation](#8-risks-and-mitigation)
9. [Success Criteria](#9-success-criteria)
10. [Appendix](#10-appendix)


## 1. Project Overview

### 1.1 Project Background

As enterprise large-model applications move from proof of concept to production deployment, the following issues have become increasingly prominent:

| Problem Area | Specific Symptoms |
|----------|----------|
| **Security Risks** | Lacks effective interception means for security threats such as prompt injection, jailbreak attacks, sensitive data leakage, and harmful content generation |
| **Fragmented Management** | Multi-model, multi-account, multi-business-line access is scattered, lacking a unified governance entry point |
| **Cost Out of Control** | Free-token accounts lack an automatic failover mechanism once rate-limited, causing service unavailability or cost spikes |
| **Protocol Fragmentation** | Different vendors such as OpenAI and Anthropic use different API protocols, forcing business teams to maintain separate integration code for each vendor |
| **Missing Audit** | Lacks end-to-end request/response logging, making it impossible to trace security events and analyze usage |
| **Quota Chaos** | Lacks unified quota management and rate limit mechanisms, leading to resource abuse and cost overruns |
| **Scattered Configuration** | Configuration is scattered across multiple files; changes require service restarts, cannot take effect in real time, and lacks operational auditing |
| **Administrative Security** | The management console lacks multi-factor authentication, exposing the risk of account compromise leading to malicious configuration tampering |
| **Database Lock-in** | Only SQLite is supported, which cannot meet high-concurrency scenarios and DBA unified operations requirements |

### 1.2 Project Goals

Build a **lightweight, self-hosted, functionally focused, fully UI-driven configuration, MFA-capable, multi-database** AI gateway system. Core capabilities include:

| No. | Capability | Description |
|------|------|------|
| 1 | **Unified Multi-Protocol Access** | Simultaneously supports the three major protocols: OpenAI Chat Completions / Responses API / Anthropic Messages API |
| 2 | **Intelligent Routing and SLB** | Supports model alias mapping, weighted load balancing, and automatic failover |
| 3 | **Model Guardrails** | Bidirectional input/output filtering, supporting keywords, regex, PII, injection detection, and extensible semantic review |
| 4 | **Quota and Rate Limiting** | Supports per-user/per-model request count and token quota configuration with scheduled automatic reset |
| 5 | **Audit Logs** | End-to-end request/response logging with multi-dimensional query and analysis; operational audit records all configuration changes |
| 6 | **Full-UI Configuration Management** | All configuration is done through the Web UI, with no need to edit config files; changes take effect in real time |
| 7 | **Configuration Hot-Reload** | Model access, guardrail dictionaries, routing rules, and other configuration changes take effect automatically without restarting the service |
| 8 | **Management Console MFA** | Supports TOTP multi-factor authentication to enhance management console security |
| 9 | **Multi-Database Support** | Simultaneously supports SQLite / PostgreSQL / MySQL to meet different deployment scale requirements |

### 1.3 Project Scope

**Included**:
- AI gateway core services (multi-protocol access, routing, SLB, guardrails, audit, quota)
- Full-featured Web management UI (all configuration done via the UI)
- Operational audit logs (recording all configuration changes)
- Management console MFA (TOTP multi-factor authentication)
- Configuration hot-reload mechanism (changes take effect automatically without restart)
- Multi-database support (SQLite / PostgreSQL / MySQL)
- RESTful management API (for UI invocation and external integration)

**Not Included**:
- User/tenant management system (initial release uses simple API Key authentication; enterprise SSO integration may be added later)
- Billing/top-up/redemption code system
- Online payment integration
- WebAuthn/Passkey (to be supported in later iterations)


## 2. User Roles and Use Cases

### 2.1 User Roles

| Role | Responsibilities | Typical Operations |
|------|------|----------|
| **System Administrator** | Gateway global configuration, user management, system operations, security policy formulation | Configure system parameters, manage API Keys, configure MFA policy, view operational audit logs |
| **AI Platform Operations** | Model management, routing configuration, quota management | Configure model providers in the UI, create model aliases, set SLB weights, configure quotas |
| **Security Administrator** | Guardrail policy formulation and management | Maintain sensitive term libraries in the UI, configure PII detection rules, manage injection rules |
| **Business Developer** | Access AI capabilities through the gateway | Obtain API Keys, view usage documentation online, view personal usage statistics |

### 2.2 Use Cases

| Scenario ID | Scenario Name | Description |
|--------|----------|------|
| S-001 | Onboard a new model provider | Operations staff click "Add Provider" in the UI, fill in the information, and save — it **takes effect immediately** without restarting the gateway |
| S-002 | Urgent sensitive term addition | After the security administrator discovers a new sensitive term, they add it immediately in the UI, and it takes effect for interception across all requests **within 3 seconds** |
| S-003 | Dynamic weight adjustment | Operations staff adjust provider weights online, and **traffic distribution changes in real time** without restart |
| S-004 | Automatic failover on rate limit | Operations staff configure the failover policy in the UI, and the system automatically switches when the upstream is rate-limited |
| S-005 | Online quota adjustment | The administrator adjusts user quotas in the UI, which **take effect immediately** without restart |
| S-006 | Audit log query | The security administrator filters interception records in the UI and exports reports for compliance review |
| S-007 | Configuration change traceability | The system administrator views the operational audit log to see who modified sensitive term configuration and when |
| S-008 | MFA device binding | After logging in, the user goes to personal account security settings and scans a QR code to bind a TOTP authenticator |
| S-009 | Database migration | Operations staff migrate from SQLite to MySQL/PostgreSQL by modifying only the connection configuration, with no business impact |


## 3. Functional Requirements

### 3.1 System Settings Module

| Requirement ID | REQ-001 |
|--------|---------|
| **Name** | System Parameter Configuration |
| **Description** | Configure gateway global runtime parameters via the UI |
| **Priority** | P0 |
| **UI Page** | System Settings > General Settings |
| **Configuration Items** | ① Gateway listening port ② Log level (debug/info/warn/error) ③ Audit log retention days (default 90 days) ④ Default timeout (seconds) ⑤ Maximum concurrent connections ⑥ Whether to enable guardrails (global switch) ⑦ Hot-reload effective time (seconds, default 3 seconds) |

| Requirement ID | REQ-002 |
|--------|---------|
| **Name** | API Key Management |
| **Description** | Create and manage API Keys for invoking the gateway, supporting independent keys for different business teams |
| **Priority** | P0 |
| **UI Page** | System Settings > API Key Management |
| **Feature Points** | ① Create API Key (auto-generated, with usage notes) ② Enable/Disable key ③ Delete key ④ View key list (key value displayed masked) ⑤ Record each key's last-used time |

| Requirement ID | REQ-003 |
|--------|---------|
| **Name** | Operational Audit Log |
| **Description** | Record all configuration change operations, support query and export, meeting compliance traceability requirements |
| **Priority** | P0 |
| **UI Page** | System Settings > Operational Audit |
| **Recorded Content** | Operator, operation time, operation type (create/modify/delete), operation module (provider/model/guardrail/quota, etc.), before/after comparison, operation IP, configuration effective status |
| **Feature Points** | ① Filter by time range ② Filter by operator ③ Filter by module ④ Export as CSV |


### 3.2 Configuration Hot-Reload Mechanism (Core Capability)

| Requirement ID | REQ-004 |
|--------|---------|
| **Name** | Configuration Hot-Reload |
| **Description** | All configuration changes (model providers, model aliases, guardrail dictionaries, quota rules, rate limits, etc.) take effect automatically after UI operations, with no need to restart the gateway process and no impact on online business |
| **Priority** | P0 |
| **Effective Time** | Within ≤ 3 seconds of a configuration change, all new requests use the new configuration; in-flight requests are not affected |

**Hot-Reload Scope**:

| Configuration Type | Change Content | Hot-Reload | Description |
|----------|----------|------------|------|
| **Provider Configuration** | Add/edit/delete providers, modify API Key, modify endpoint | ✅ Yes | Immediately affects request routing for that provider |
| **Model Alias** | Add/edit/delete aliases, modify weights, add/remove upstreams | ✅ Yes | New requests immediately use the new routing configuration |
| **SLB Weights** | Adjust weight values | ✅ Yes | Subsequent requests are distributed per the new weights |
| **Guardrail Sensitive Terms** | Add/edit/delete sensitive terms | ✅ Yes | New requests immediately use the new term library; existing streaming requests are not affected |
| **PII Rules** | Add/edit/delete PII rules | ✅ Yes | New requests immediately use the new rules |
| **Injection Rules** | Add/edit/delete injection rules | ✅ Yes | New requests immediately use the new rules |
| **Output Filtering** | Modify safety prompt template, response policy | ✅ Yes | New requests immediately use the new policy |
| **Quota Rules** | Add/edit/delete quotas, adjust limits | ✅ Yes | New requests immediately use the new quotas |
| **Rate Limits** | Add/edit/delete rate limit rules | ✅ Yes | New requests immediately use the new rate limit policy |
| **System Parameters** | Modify log level, timeout, etc. | ✅ Yes | Depending on parameter type, some take effect immediately, some take effect on next startup |

| Requirement ID | REQ-004A |
|--------|----------|
| **Name** | Hot-Reload Status Visualization |
| **Description** | Display the current configuration load status in the UI; if loading fails, provide detailed error information and rollback guidance |
| **Priority** | P1 |
| **UI Page** | System Settings > Configuration Status |
| **Displayed Content** | ① Current configuration version number ② Last load time ③ Configuration load status (normal/abnormal) ④ If abnormal, display failure reason and affected modules ⑤ Support one-click rollback to the previous configuration version |


### 3.3 Model Provider Management Module

| Requirement ID | REQ-005 |
|--------|---------|
| **Name** | Provider Management |
| **Description** | Manage access information for upstream AI model providers, including endpoints, authentication keys, protocol types, etc. |
| **Priority** | P0 |
| **UI Page** | Model Management > Provider Management |
| **Managed Fields** | ① Provider name (unique identifier) ② Protocol type (OpenAI Chat / OpenAI Responses / Anthropic / Custom) ③ API endpoint URL ④ API Key (encrypted storage, masked on input) ⑤ Whether enabled ⑥ Remarks |
| **Feature Points** | ① Create provider ② Edit provider ③ Enable/Disable ④ Delete (check whether referenced by model aliases) ⑤ Test connection ⑥ Hot-reload takes effect after change |

### 3.4 Model Routing and SLB Module

| Requirement ID | REQ-006 |
|--------|---------|
| **Name** | Model Alias Management |
| **Description** | Create business-side model aliases, mapping one alias to multiple upstream providers to achieve a unified entry point and load balancing |
| **Priority** | P0 |
| **UI Page** | Model Management > Model Aliases |
| **Managed Fields** | ① Alias name (e.g., "gpt-4") ② Associated provider list ③ Weight per provider (integer, supports dynamic adjustment) ④ Whether enabled ⑤ Remarks |
| **Feature Points** | ① Create alias ② Edit alias ③ Enable/Disable ④ Delete ⑤ View alias invocation statistics ⑥ Hot-reload takes effect after change |

| Requirement ID | REQ-007 |
|--------|---------|
| **Name** | Failover Policy Configuration |
| **Description** | Configure the automatic failover behavior when an upstream provider fails |
| **Priority** | P0 |
| **UI Page** | Model Management > Failover Settings |
| **Configuration Items** | ① Whether to enable automatic failover ② Retry count ③ Retry interval strategy (fixed/exponential backoff) ④ Trigger conditions ⑤ Circuit breaker threshold ⑥ Circuit breaker recovery time |


### 3.5 Guardrail Management Module

#### 3.5.1 Sensitive Term Management

| Requirement ID | REQ-008 |
|--------|---------|
| **Name** | Sensitive Term Management |
| **Description** | Manage the sensitive term library via the UI, supporting classification, batch operations, import and export |
| **Priority** | P0 |
| **UI Page** | Guardrail Management > Sensitive Terms |
| **Managed Fields** | ① Keyword text ② Category (political/pornographic/violent/illegal/discriminatory/custom) ③ Match mode (exact/contains/regex) ④ Action (block/alert/log-only) ⑤ Enabled status ⑥ Creation time |
| **Feature Points** | ① Add single ② Batch import ③ Export ④ Filter by category ⑤ Search ⑥ Edit ⑦ Enable/Disable ⑧ Delete ⑨ Test tool ⑩ Hot-reload takes effect after change |

#### 3.5.2 PII Rule Management

| Requirement ID | REQ-009 |
|--------|---------|
| **Name** | PII Recognition Rule Management |
| **Description** | Configure recognition rules and handling policies for personally identifiable information |
| **Priority** | P0 |
| **UI Page** | Guardrail Management > PII Rules |
| **Feature Points** | ① Create/Edit/Delete rules ② Enable/Disable ③ Built-in common rule templates ④ Test tool ⑤ Hot-reload takes effect after change |

#### 3.5.3 Injection Attack Rule Management

| Requirement ID | REQ-010 |
|--------|---------|
| **Name** | Injection Attack Rule Management |
| **Description** | Configure detection rules for prompt injection and jailbreak attacks |
| **Priority** | P0 |
| **UI Page** | Guardrail Management > Injection Rules |
| **Feature Points** | ① Create/Edit/Delete rules ② Enable/Disable ③ Built-in common rule templates ④ Hot-reload takes effect after change |

#### 3.5.4 Output Filtering Management

| Requirement ID | REQ-011 |
|--------|---------|
| **Name** | Output Filtering Configuration |
| **Description** | Configure filtering policies for model output content |
| **Priority** | P0 |
| **UI Page** | Guardrail Management > Output Filtering |
| **Configuration Items** | ① Whether to enable output detection ② Violation response policy ③ Custom safety prompt template ④ Streaming detection threshold |


### 3.6 Quota and Rate Limit Module

#### 3.6.1 Quota Management

| Requirement ID | REQ-012 |
|--------|---------|
| **Name** | Quota Management |
| **Description** | Configure call quotas per user and model, supporting scheduled automatic reset |
| **Priority** | P0 |
| **UI Page** | Quota Management > Quota List |
| **Managed Fields** | ① User identifier ② Model alias ③ Quota type ④ Period type ⑤ Quota limit ⑥ Over-limit policy ⑦ Degradation target ⑧ Enabled status |
| **Feature Points** | ① Create/Edit/Delete quotas ② Enable/Disable ③ Manual reset ④ View real-time usage ⑤ Batch create ⑥ Hot-reload takes effect after change |

#### 3.6.2 Rate Limit Management

| Requirement ID | REQ-013 |
|--------|---------|
| **Name** | Rate Limit Management |
| **Description** | Configure short-period rate limits to prevent traffic spikes |
| **Priority** | P0 |
| **UI Page** | Quota Management > Rate Limit |
| **Feature Points** | ① Create/Edit/Delete rate limit rules ② Enable/Disable ③ View rate limit hit statistics ④ Hot-reload takes effect after change |

#### 3.6.3 Quota Alerting

| Requirement ID | REQ-014 |
|--------|---------|
| **Name** | Quota Alerting Configuration |
| **Description** | Configure alert notifications when quota usage reaches a threshold |
| **Priority** | P1 |
| **UI Page** | Quota Management > Alert Settings |
| **Configuration Items** | ① Alert threshold (e.g., 80%, 95%) ② Notification method (in-page notification/email/Webhook) ③ Notification recipient |


### 3.7 Audit Log Module

| Requirement ID | REQ-015 |
|--------|---------|
| **Name** | Invocation Audit Log Query |
| **Description** | Query all API invocation records, supporting multi-dimensional filtering and export |
| **Priority** | P0 |
| **UI Page** | Audit Logs > Invocation Logs |
| **Displayed Fields** | Request ID, time, user, model alias, actual upstream, input (masked), output (masked), token usage, status, latency, whether blocked, block reason |
| **Feature Points** | ① Filter by time range ② Filter by user ③ Filter by model ④ Filter by block status ⑤ Filter by block category ⑥ View details ⑦ Export |

### 3.8 Dashboard and Statistics Module

| Requirement ID | REQ-016 |
|--------|---------|
| **Name** | Overview Dashboard |
| **Description** | Display the overall status and key metrics of gateway operation |
| **Priority** | P1 |
| **UI Page** | Home/Dashboard |
| **Displayed Content** | ① Today's total requests, success count, block count ② 7-day call trend chart ③ Model call volume ranking ④ Block category distribution pie chart ⑤ Upstream health status overview ⑥ Quota usage overview ⑦ System running status |


### 3.9 Multi-Protocol Support

| Requirement ID | REQ-017 |
|--------|---------|
| **Name** | Multi-Protocol Compatible Access |
| **Description** | The gateway simultaneously supports three core protocols: OpenAI Chat Completions API, OpenAI Responses API, Anthropic Messages API |
| **Priority** | P0 |
| **Acceptance Criteria** | Standard SDKs can directly invoke the corresponding endpoints; the three protocols share routing, guardrail, SLB, and audit capabilities |


### 3.10 System Operations Module

| Requirement ID | REQ-018 |
|--------|---------|
| **Name** | System Status Monitoring |
| **Description** | View gateway running status and performance metrics |
| **Priority** | P1 |
| **UI Page** | System Operations > Status Monitoring |
| **Displayed Content** | ① Service running status ② Memory/CPU usage ③ Current connections ④ Upstream health status ⑤ Per-model real-time QPS ⑥ Recent error logs ⑦ Configuration load status |

| Requirement ID | REQ-019 |
|--------|---------|
| **Name** | Data Backup and Recovery |
| **Description** | Support backing up and restoring all configuration data via the UI |
| **Priority** | P2 |
| **UI Page** | System Operations > Data Backup |
| **Feature Points** | ① Manual backup (includes all configuration table data) ② Restore from backup file ③ View backup history |


### 3.11 System Security Module (MFA)

#### 3.11.1 MFA Global Configuration

| Requirement ID | REQ-020 |
|--------|---------|
| **Name** | MFA Global Configuration |
| **Description** | Administrators can enable MFA in system settings and choose whether to enforce it for all users |
| **Priority** | P1 |
| **UI Page** | System Settings > Security Settings > MFA Configuration |
| **Configuration Items** | ① Whether to enable MFA ② Whether to enforce for all users ③ Supported verification methods (TOTP) ④ Number of backup recovery codes (default 10) ⑤ Enforcement grace period (days) |
| **Hot-Reload** | ✅ Takes effect immediately after configuration change |

#### 3.11.2 User MFA Self-Service Binding

| Requirement ID | REQ-021 |
|--------|---------|
| **Name** | User MFA Self-Service Binding |
| **Description** | Users can self-bind MFA devices in personal account settings |
| **Priority** | P1 |
| **UI Page** | Personal Account > Security Settings > Bind MFA |
| **Feature Points** | ① Generate and display QR code ② User enters dynamic verification code to complete verification ③ Generate backup recovery codes and prompt to save them |

#### 3.11.3 MFA Login Verification Flow

| Requirement ID | REQ-022 |
|--------|---------|
| **Name** | MFA Login Verification Flow |
| **Description** | Users who have bound MFA must enter a dynamic verification code after password verification |
| **Priority** | P1 |
| **Feature Points** | ① Password verification → MFA status check → redirect to MFA verification page ② Retry on incorrect code; after 5 consecutive failures, lock the account ③ Support login with backup recovery codes |

#### 3.11.4 MFA Device Management

| Requirement ID | REQ-023 |
|--------|---------|
| **Name** | MFA Device Management |
| **Description** | Administrators and users manage MFA devices separately |
| **Priority** | P1 |
| **Feature Points** | ① Administrator: view all users' MFA status, force unbind (recorded in audit) ② User: view bound devices, unbind, rebind |

#### 3.11.5 MFA Operation Audit

| Requirement ID | REQ-024 |
|--------|---------|
| **Name** | MFA Operation Audit |
| **Description** | All MFA-related operations are recorded in the operational audit log |
| **Priority** | P1 |
| **Recorded Content** | Operator, operation time, operation type, operation IP |


## 4. UI Layout Planning

### 4.1 Overall Layout

```
+----------------------------------------------------------+
|  AI Gateway Admin   [Logo]                    [Avatar]   |
+--------+-------------------------------------------------+
| Nav    |  Content area                                  |
|        |                                                 |
| ├ Home |  [Breadcrumb]                                  |
| ├ Models |                                                 |
| │ ├ Providers |  [Page title]                            |
| │ └ Aliases  |                                                 |
| ├ Guards |                                                 |
| │ ├ Sensitive Words |                                                 |
| │ ├ PII Rules  |                                                 |
| │ ├ Injection Rules |                                                 |
| │ └ Output Filter |                                                 |
| ├ Quotas |                                                 |
| │ ├ Quota List |                                                 |
| │ ├ Rate Limits |                                                 |
| │ └ Alerts     |                                                 |
| ├ Audit Logs |                                                 |
| │ ├ Call Logs  |                                                 |
| │ └ Operation Audit |                                                 |
| └ System     |                                                 |
|   ├ General  |                                                 |
|   ├ API Keys |                                                 |
|   ├ Security  |  ← MFA config                                 |
|   ├ Config Status |  ← hot-reload status                       |
|   ├ Monitor   |                                                 |
|   └ Backup    |                                                 |
+--------+-------------------------------------------------+
```

### 4.2 Page Feature Checklist

| Page | Route | Core Features |
|------|------|----------|
| Home/Dashboard | `/` | Key metric cards, trend charts, rankings, status overview |
| Provider Management | `/providers` | List display, CRUD, test connection, enable/disable, hot-reload status |
| Model Alias Management | `/models` | List display, CRUD, weight configuration, call statistics |
| Failover Settings | `/models/failover` | Policy configuration form |
| Sensitive Term Management | `/guard/keywords` | List display, CRUD, category filtering, batch import/export, test tool |
| PII Rule Management | `/guard/pii` | Rule list, CRUD, built-in templates, test tool |
| Injection Rule Management | `/guard/injection` | Rule list, CRUD, built-in templates |
| Output Filtering Configuration | `/guard/output` | Configuration form |
| Quota Management | `/quota` | Quota list, CRUD, progress display, manual reset |
| Rate Limit | `/quota/ratelimit` | Rule list, CRUD |
| Alert Settings | `/quota/alert` | Configuration form |
| Invocation Audit Log | `/audit/calls` | List display, multi-dimensional filtering, detail view, export |
| Operational Audit Log | `/audit/operations` | List display, filtering, export |
| General Settings | `/settings/general` | Configuration form |
| API Key Management | `/settings/apikeys` | List display, create, enable/disable, delete |
| Security Settings | `/settings/security` | MFA global configuration, MFA device management |
| Configuration Status | `/settings/config-status` | Configuration version, load status, load log, one-click rollback |
| Status Monitoring | `/settings/status` | Real-time status cards, monitoring data |
| Data Backup | `/settings/backup` | Backup/restore operations |
| Personal Account > Security Settings | `/account/security` | MFA bind/unbind, recovery code management |

### 4.3 Hot-Reload Status Page Illustration

```
┌─────────────────────────────────────────────────────────────────────┐
│  System > Config Status                                              │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Current config version: v20260902.143022                           │
│  Last loaded at:      2026-09-02 14:30:22                           │
│  Load status: ✅ OK                                                  │
│  Active modules: Providers(12) Aliases(8) Sensitive(156) PII(6)     │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  📋 Recent load log                                         │   │
│  │  14:30:22  [ok] Providers loaded (12 items)                 │   │
│  │  14:30:22  [ok] Model aliases loaded (8 items)              │   │
│  │  14:30:22  [ok] Sensitive words loaded (156 items, AC built)│   │
│  │  14:25:10  [ok] Quota rules loaded (23 items)               │   │
│  │  14:20:05  [ok] Provider "deepseek-account-3" added          │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  [Reload config]  [Export config]  [Rollback to previous]           │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```


## 5. Non-Functional Requirements

| Requirement ID | Category | Description | Target Value |
|--------|------|------|--------|
| NFR-001 | Performance | End-to-end latency increase for basic guardrail version | ≤ 100ms |
| NFR-002 | Performance | Latency increase for enhanced guardrail version (with semantic review) | ≤ 300ms |
| NFR-003 | Performance | Single-instance QPS | ≥ 1000 |
| NFR-004 | Availability | Gateway availability | ≥ 99.9% |
| NFR-005 | Availability | Automatic degradation when guardrail service fails | Does not block business |
| NFR-006 | Accuracy | Sensitive term hit rate | ≥ 95% |
| NFR-007 | Accuracy | Sensitive term false positive rate | ≤ 1% |
| NFR-008 | Security | API Key transmission | Over HTTPS only |
| NFR-009 | Security | Audit log PII | Auto-masked storage |
| NFR-010 | Security | Management console MFA | Supports TOTP |
| NFR-011 | Scalability | Adding protocol adapters | Only requires parser and serializer implementation |
| NFR-012 | Usability | Configuration effective method | UI operation takes effect immediately, hot-reload ≤ 3 seconds |
| NFR-013 | Observability | Prometheus metrics | Full coverage |
| NFR-014 | UI Response | Management UI operation response | ≤ 1 second |
| NFR-015 | Concurrency | UI management operation concurrency | Supports 10 concurrent operators |
| NFR-016 | Database | Supported database types | SQLite / PostgreSQL / MySQL |


## 6. Technical Implementation

### 6.1 Overall Architecture

The gateway adopts a **front-end/back-end separated** architecture, with the back-end as a single-process Go service (with built-in front-end static assets), and data storage supporting three databases: SQLite / PostgreSQL / MySQL.

**Architectural Layers**:

| Layer | Responsibility | Core Components |
|------|------|----------|
| **Access Layer** | Receives HTTP requests, supports three protocol endpoints + management API | Gin HTTP Server |
| **Middleware Chain** | Sequential execution of authentication, guardrails, routing, audit | Authentication middleware, guardrail middleware, routing middleware, audit middleware |
| **Protocol Adaptation Layer** | Request parsing and response serialization, automatic protocol detection | Protocol detector, canonical model converter |
| **Core Business Layer** | SLB, guardrail detection, quota management | Load balancer, guardrail engine, quota manager |
| **Storage Layer** | Audit log, configuration data, quota data persistence | SQLite / PostgreSQL / MySQL |
| **Management Service Layer** | Management API invoked by the UI, configuration hot-update notifications | Management API Handler, configuration change notification mechanism |

### 6.2 Technology Selection

| Component | Selection | Description |
|------|------|------|
| Programming Language | Go 1.21+ | Single binary, excellent concurrency model |
| Web Framework | Gin | Lightweight, good performance, mature middleware ecosystem |
| Management API | RESTful | For front-end UI invocation |
| Front-end Framework | **React 18+** | Component-based, mature ecosystem, comprehensive TypeScript support |
| Front-end UI Component Library | **Ant Design 5.x** | Enterprise-grade React UI component library, out-of-the-box |
| Front-end State Management | **Zustand** or **TanStack Query** | Lightweight state management + server state caching |
| Front-end Routing | **React Router v6** | Declarative routing |
| Front-end Build | **Vite** | Fast builds |
| Front-end Deployment | Embedded in Go binary | Bundles build artifacts via embed for single-file delivery |
| Configuration Storage | Database (not config files) | All configuration stored in the database, supporting hot-reload |
| ORM | GORM | Simplifies database operations, supports multiple databases |
| Database Driver | GORM driver | Unified interface for SQLite / PostgreSQL / MySQL |
| TOTP Library | pquerna/otp | MFA verification code generation and verification |
| QR Code Generation | skip2/go-qrcode | MFA binding QR code generation |

### 6.3 Core Module Design

#### 6.3.1 Multi-Database Support

| Database | Applicable Scenario | Connection Configuration Example |
|--------|----------|-------------|
| **SQLite** | Development/testing, small-scale deployment, single-machine environments | `sqlite:./gateway.db` |
| **PostgreSQL** | Production, high-concurrency scenarios | `postgres://user:pass@localhost:5432/gateway` |
| **MySQL** | Production, enterprise unified database stack | `mysql://user:pass@tcp(localhost:3306)/gateway` |

**Database Switching Method**:
- Configured via the `DB_TYPE` and `DB_DSN` environment variables
- Database migration (table creation, indexes) runs automatically on first startup
- Supports database version migration management (GORM AutoMigrate + manual migration scripts)

#### 6.3.2 Configuration Hot-Reload Mechanism

All configuration is stored in the database, and changes take effect in real time triggered via the management API:

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Config hot-reload flow                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  1. User submits a config change in the UI (e.g. add a word)        │
│     ↓                                                              │
│  2. Admin API writes the change to the database                     │
│     ↓                                                              │
│  3. Config-change event fired (DB trigger / message / polling)      │
│     ↓                                                              │
│  4. Core gateway receives the change notification                    │
│     ↓                                                              │
│  5. Reloads the affected module config from the database            │
│     ↓                                                              │
│  6. Atomically swaps the in-memory cache (new config on next request)│
│     ↓                                                              │
│  7. Change recorded in the operation-audit log                      │
│     ↓                                                              │
│  8. Load status updated to the config-status table for the UI       │
│                                                                     │
│  Total time: ≤ 3 seconds                                            │
│  In-flight requests: unaffected (completed with old config)         │
│  New requests: use the new config immediately                       │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

**Hot-Reload Trigger Methods**:

| Method | Description | Applicable Scenario |
|------|------|----------|
| **Real-time Trigger** | Actively pushes a notification after a configuration change (via channel or message queue) | Recommended, lowest latency |
| **Scheduled Polling** | The gateway queries the database configuration version number every N seconds and reloads if changes are detected | Fallback option, simple to implement |
| **Manual Trigger** | User clicks the "Reload Configuration" button in the UI | Emergency scenarios |


## 7. Deployment Plan

### 7.1 Deployment Methods

| Method | Applicable Scenario | Description |
|------|----------|------|
| **Single Binary** | Development/testing, small-scale | Run directly, with built-in SQLite |
| **Docker Container** | Standardized delivery | Management UI embedded in the image |
| **Kubernetes** | Production environment | Multiple replicas, ConfigMap, Secret |

### 7.2 Database Selection Recommendations

| Scenario | Recommended Database | Description |
|------|-----------|------|
| Development/Testing | SQLite | Zero configuration, quick start |
| Daily requests < 100k | SQLite | Sufficient performance |
| Daily requests > 100k | PostgreSQL / MySQL | High-concurrency read/write support |
| Enterprise unified database stack | MySQL / PostgreSQL | Meets DBA operations standards |

### 7.3 First-Startup Onboarding

- Automatically initializes the database on first startup
- Prompts for the management UI address
- Guides the user to create an administrator account and default API Key


## 8. Risks and Mitigation

| Risk | Impact | Probability | Mitigation Measures |
|------|------|------|----------|
| Insufficient front-end development resources | UI progress delay | Medium | Adopt mature Ant Design components; reuse open-source admin templates; consider hiring professional React developers |
| Frequent changes to upstream model protocols | Adapters require continuous maintenance | Medium | Adapter pattern isolates changes; prioritize support for mainstream protocols |
| High guardrail false positive rate | Affects normal business | Medium | Support flexible configuration of three actions; provide test tools in the UI |
| Audit log data volume growth | Storage pressure | Medium | SQLite suitable for daily requests < 100k; provide automatic cleanup strategy |
| High semantic review model latency | Affects user experience | Medium | Degrade to basic rules; support per-request granularity toggle |
| UI configuration change concurrency conflicts | Configuration overwrites | Low | Database transaction isolation; optimistic locking mechanism |
| React front-end and back-end embed integration | Complex build process | Low | Use standard go:embed; CI/CD automated build |


## 9. Success Criteria

| Dimension | Metric | Target |
|------|------|------|
| **Functional Completeness** | P0 requirement delivery rate | 100% |
| **Configuration Efficiency** | Time from new provider to effective | ≤ 3 minutes |
| **Performance** | End-to-end latency increase (basic version) | ≤ 100ms |
| **Accuracy** | Sensitive term hit rate | ≥ 95% |
| **Accuracy** | Sensitive term false positive rate | ≤ 1% |
| **Availability** | Gateway availability | ≥ 99.9% |
| **Maintainability** | Configuration change effective time | ≤ 3 seconds |
| **User Satisfaction** | Operations efficiency improvement | ≥ 50% |


## 10. Appendix

### 10.1 Glossary

| Term | Description |
|------|------|
| **Guardrails** | Mechanism for performing security detection and filtering on model inputs and outputs |
| **SLB** | Service Load Balancing; distributes requests across multiple upstreams |
| **PII** | Personally Identifiable Information, e.g., phone numbers, ID numbers |
| **Prompt Injection** | Malicious users crafting prompts to attack the model |
| **Canonical Model** | The unified internal request/response data format of the gateway |
| **Quota** | Long-term total limit, reset by day/week/month |
| **Rate Limit** | Short-term rate limit, sliding window by second/minute |
| **Provider** | An access account for an upstream AI model service |
| **Model Alias** | A unified model name called by the business side, which can map to multiple providers |
| **Hot Reload** | Configuration changes take effect without restarting the service |
| **MFA (Multi-Factor Authentication)** | Multi-Factor Authentication; this system uses TOTP |

### 10.2 Reference Documents

| Document | Description |
|------|------|
| OpenAI Chat Completions API | https://platform.openai.com/docs/api-reference/chat |
| OpenAI Responses API | https://platform.openai.com/docs/api-reference/responses |
| Anthropic Messages API | https://docs.anthropic.com/en/api/messages |
| Qwen3Guard | Alibaba Cloud open-source safety review model |
| Ant Design React | https://ant.design/ |
| React Router | https://reactrouter.com/ |
| TanStack Query | https://tanstack.com/query |


**End of Document**
