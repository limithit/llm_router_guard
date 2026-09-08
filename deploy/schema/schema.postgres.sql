--
-- PostgreSQL database dump
--


-- Dumped from database version 17.10 (Debian 17.10-0+deb13u1)
-- Dumped by pg_dump version 17.10 (Debian 17.10-0+deb13u1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: admin_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_users (
    id bigint NOT NULL,
    username character varying(64),
    password_hash character varying(128),
    mfa_secret character varying(128),
    mfa_enabled boolean,
    mfa_bound_at timestamp with time zone,
    recovery_codes_json text,
    failed_logins bigint,
    locked_until timestamp with time zone,
    last_login_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: admin_users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.admin_users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.admin_users_id_seq OWNED BY public.admin_users.id;


--
-- Name: alias_upstreams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.alias_upstreams (
    id bigint NOT NULL,
    alias_id bigint,
    provider_id bigint,
    upstream_model character varying(64),
    weight bigint
);


--
-- Name: alias_upstreams_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.alias_upstreams_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: alias_upstreams_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.alias_upstreams_id_seq OWNED BY public.alias_upstreams.id;


--
-- Name: api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_keys (
    id bigint NOT NULL,
    name character varying(64),
    key_hash character varying(64),
    key_masked character varying(64),
    remark character varying(255),
    allowed_models_json text,
    ip_allowlist_json text,
    ip_allowlist_enabled boolean,
    enabled boolean,
    last_used_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: api_keys_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.api_keys_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: api_keys_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.api_keys_id_seq OWNED BY public.api_keys.id;


--
-- Name: backup_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.backup_records (
    id bigint NOT NULL,
    filename character varying(128),
    content text,
    size_bytes bigint,
    created_at timestamp with time zone
);


--
-- Name: backup_records_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.backup_records_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: backup_records_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.backup_records_id_seq OWNED BY public.backup_records.id;


--
-- Name: call_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.call_logs (
    id bigint NOT NULL,
    request_id character varying(40),
    created_at timestamp with time zone,
    api_key_id bigint,
    api_key_label character varying(64),
    protocol character varying(32),
    model_alias character varying(64),
    upstream_provider character varying(64),
    upstream_model character varying(64),
    input_text text,
    output_text text,
    prompt_tokens bigint,
    completion_tokens bigint,
    latency_ms bigint,
    status character varying(24),
    blocked boolean,
    block_category character varying(64),
    block_reason character varying(512),
    error_msg character varying(512),
    guard_findings text
);


--
-- Name: call_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.call_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: call_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.call_logs_id_seq OWNED BY public.call_logs.id;


--
-- Name: config_load_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.config_load_logs (
    id bigint NOT NULL,
    "time" timestamp with time zone,
    module character varying(32),
    status character varying(16),
    message character varying(512)
);


--
-- Name: config_load_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.config_load_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: config_load_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.config_load_logs_id_seq OWNED BY public.config_load_logs.id;


--
-- Name: config_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.config_versions (
    id bigint NOT NULL,
    version character varying(32),
    snapshot_json text,
    status character varying(16),
    message character varying(512),
    created_at timestamp with time zone
);


--
-- Name: config_versions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.config_versions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: config_versions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.config_versions_id_seq OWNED BY public.config_versions.id;


--
-- Name: guard_keywords; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.guard_keywords (
    id bigint NOT NULL,
    word character varying(255),
    category character varying(32),
    match_mode character varying(16),
    action character varying(16),
    enabled boolean,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: guard_keywords_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.guard_keywords_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: guard_keywords_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.guard_keywords_id_seq OWNED BY public.guard_keywords.id;


--
-- Name: injection_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.injection_rules (
    id bigint NOT NULL,
    name character varying(64),
    pattern character varying(512),
    match_mode character varying(16),
    action character varying(16),
    enabled boolean,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: injection_rules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.injection_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: injection_rules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.injection_rules_id_seq OWNED BY public.injection_rules.id;


--
-- Name: model_aliases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.model_aliases (
    id bigint NOT NULL,
    alias character varying(64),
    enabled boolean,
    remark character varying(255),
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: model_aliases_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.model_aliases_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: model_aliases_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.model_aliases_id_seq OWNED BY public.model_aliases.id;


--
-- Name: operation_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operation_logs (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    operator character varying(64),
    action character varying(24),
    module character varying(32),
    target character varying(128),
    before_json text,
    after_json text,
    ip character varying(64),
    effective boolean
);


--
-- Name: operation_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.operation_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: operation_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.operation_logs_id_seq OWNED BY public.operation_logs.id;


--
-- Name: pii_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pii_rules (
    id bigint NOT NULL,
    name character varying(64),
    category character varying(32),
    pattern character varying(512),
    replacement character varying(64),
    action character varying(16),
    enabled boolean,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: pii_rules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.pii_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: pii_rules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.pii_rules_id_seq OWNED BY public.pii_rules.id;


--
-- Name: providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.providers (
    id bigint NOT NULL,
    name character varying(64),
    protocol character varying(32),
    base_url character varying(255),
    api_key_enc text,
    api_key_masked character varying(64),
    enabled boolean,
    remark character varying(255),
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: providers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.providers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: providers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.providers_id_seq OWNED BY public.providers.id;


--
-- Name: quota; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.quota (
    id bigint NOT NULL,
    api_key_id bigint,
    model_alias character varying(64),
    quota_type character varying(16),
    period character varying(16),
    limit_value bigint,
    used_value bigint,
    reset_at timestamp with time zone,
    over_action character varying(16),
    degrade_alias character varying(64),
    enabled boolean,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: quota_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.quota_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: quota_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.quota_id_seq OWNED BY public.quota.id;


--
-- Name: rate_limit_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rate_limit_rules (
    id bigint NOT NULL,
    api_key_id bigint,
    model_alias character varying(64),
    window_seconds bigint,
    max_requests bigint,
    total_hits bigint,
    enabled boolean,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);


--
-- Name: rate_limit_rules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.rate_limit_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: rate_limit_rules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.rate_limit_rules_id_seq OWNED BY public.rate_limit_rules.id;


--
-- Name: system_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.system_settings (
    key character varying(64) NOT NULL,
    value_json text,
    updated_at timestamp with time zone
);


--
-- Name: admin_users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_users ALTER COLUMN id SET DEFAULT nextval('public.admin_users_id_seq'::regclass);


--
-- Name: alias_upstreams id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alias_upstreams ALTER COLUMN id SET DEFAULT nextval('public.alias_upstreams_id_seq'::regclass);


--
-- Name: api_keys id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys ALTER COLUMN id SET DEFAULT nextval('public.api_keys_id_seq'::regclass);


--
-- Name: backup_records id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.backup_records ALTER COLUMN id SET DEFAULT nextval('public.backup_records_id_seq'::regclass);


--
-- Name: call_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.call_logs ALTER COLUMN id SET DEFAULT nextval('public.call_logs_id_seq'::regclass);


--
-- Name: config_load_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.config_load_logs ALTER COLUMN id SET DEFAULT nextval('public.config_load_logs_id_seq'::regclass);


--
-- Name: config_versions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.config_versions ALTER COLUMN id SET DEFAULT nextval('public.config_versions_id_seq'::regclass);


--
-- Name: guard_keywords id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guard_keywords ALTER COLUMN id SET DEFAULT nextval('public.guard_keywords_id_seq'::regclass);


--
-- Name: injection_rules id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.injection_rules ALTER COLUMN id SET DEFAULT nextval('public.injection_rules_id_seq'::regclass);


--
-- Name: model_aliases id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_aliases ALTER COLUMN id SET DEFAULT nextval('public.model_aliases_id_seq'::regclass);


--
-- Name: operation_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operation_logs ALTER COLUMN id SET DEFAULT nextval('public.operation_logs_id_seq'::regclass);


--
-- Name: pii_rules id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pii_rules ALTER COLUMN id SET DEFAULT nextval('public.pii_rules_id_seq'::regclass);


--
-- Name: providers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.providers ALTER COLUMN id SET DEFAULT nextval('public.providers_id_seq'::regclass);


--
-- Name: quota id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota ALTER COLUMN id SET DEFAULT nextval('public.quota_id_seq'::regclass);


--
-- Name: rate_limit_rules id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rate_limit_rules ALTER COLUMN id SET DEFAULT nextval('public.rate_limit_rules_id_seq'::regclass);


--
-- Name: admin_users admin_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_users
    ADD CONSTRAINT admin_users_pkey PRIMARY KEY (id);


--
-- Name: alias_upstreams alias_upstreams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alias_upstreams
    ADD CONSTRAINT alias_upstreams_pkey PRIMARY KEY (id);


--
-- Name: api_keys api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);


--
-- Name: backup_records backup_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.backup_records
    ADD CONSTRAINT backup_records_pkey PRIMARY KEY (id);


--
-- Name: call_logs call_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.call_logs
    ADD CONSTRAINT call_logs_pkey PRIMARY KEY (id);


--
-- Name: config_load_logs config_load_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.config_load_logs
    ADD CONSTRAINT config_load_logs_pkey PRIMARY KEY (id);


--
-- Name: config_versions config_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.config_versions
    ADD CONSTRAINT config_versions_pkey PRIMARY KEY (id);


--
-- Name: guard_keywords guard_keywords_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guard_keywords
    ADD CONSTRAINT guard_keywords_pkey PRIMARY KEY (id);


--
-- Name: injection_rules injection_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.injection_rules
    ADD CONSTRAINT injection_rules_pkey PRIMARY KEY (id);


--
-- Name: model_aliases model_aliases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_aliases
    ADD CONSTRAINT model_aliases_pkey PRIMARY KEY (id);


--
-- Name: operation_logs operation_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operation_logs
    ADD CONSTRAINT operation_logs_pkey PRIMARY KEY (id);


--
-- Name: pii_rules pii_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pii_rules
    ADD CONSTRAINT pii_rules_pkey PRIMARY KEY (id);


--
-- Name: providers providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.providers
    ADD CONSTRAINT providers_pkey PRIMARY KEY (id);


--
-- Name: quota quota_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota
    ADD CONSTRAINT quota_pkey PRIMARY KEY (id);


--
-- Name: rate_limit_rules rate_limit_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rate_limit_rules
    ADD CONSTRAINT rate_limit_rules_pkey PRIMARY KEY (id);


--
-- Name: system_settings system_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_pkey PRIMARY KEY (key);


--
-- Name: idx_admin_users_username; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_admin_users_username ON public.admin_users USING btree (username);


--
-- Name: idx_alias_upstreams_alias_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_alias_upstreams_alias_id ON public.alias_upstreams USING btree (alias_id);


--
-- Name: idx_alias_upstreams_provider_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_alias_upstreams_provider_id ON public.alias_upstreams USING btree (provider_id);


--
-- Name: idx_api_keys_key_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_key_hash ON public.api_keys USING btree (key_hash);


--
-- Name: idx_api_keys_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_api_keys_name ON public.api_keys USING btree (name);


--
-- Name: idx_call_logs_api_key_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_call_logs_api_key_id ON public.call_logs USING btree (api_key_id);


--
-- Name: idx_call_logs_blocked; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_call_logs_blocked ON public.call_logs USING btree (blocked);


--
-- Name: idx_call_logs_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_call_logs_created_at ON public.call_logs USING btree (created_at);


--
-- Name: idx_call_logs_model_alias; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_call_logs_model_alias ON public.call_logs USING btree (model_alias);


--
-- Name: idx_call_logs_request_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_call_logs_request_id ON public.call_logs USING btree (request_id);


--
-- Name: idx_call_logs_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_call_logs_status ON public.call_logs USING btree (status);


--
-- Name: idx_config_load_logs_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_config_load_logs_time ON public.config_load_logs USING btree ("time");


--
-- Name: idx_config_versions_version; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_config_versions_version ON public.config_versions USING btree (version);


--
-- Name: idx_guard_keywords_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_guard_keywords_category ON public.guard_keywords USING btree (category);


--
-- Name: idx_guard_keywords_word; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_guard_keywords_word ON public.guard_keywords USING btree (word);


--
-- Name: idx_model_aliases_alias; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_model_aliases_alias ON public.model_aliases USING btree (alias);


--
-- Name: idx_operation_logs_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operation_logs_created_at ON public.operation_logs USING btree (created_at);


--
-- Name: idx_operation_logs_module; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operation_logs_module ON public.operation_logs USING btree (module);


--
-- Name: idx_operation_logs_operator; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_operation_logs_operator ON public.operation_logs USING btree (operator);


--
-- Name: idx_pii_rules_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pii_rules_category ON public.pii_rules USING btree (category);


--
-- Name: idx_providers_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_providers_name ON public.providers USING btree (name);


--
-- Name: idx_quota_api_key_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_api_key_id ON public.quota USING btree (api_key_id);


--
-- Name: idx_quota_model_alias; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_model_alias ON public.quota USING btree (model_alias);


--
-- Name: idx_rate_limit_rules_api_key_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rate_limit_rules_api_key_id ON public.rate_limit_rules USING btree (api_key_id);


--
-- Name: idx_rate_limit_rules_model_alias; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rate_limit_rules_model_alias ON public.rate_limit_rules USING btree (model_alias);


--
-- Name: idx_system_settings_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_system_settings_updated_at ON public.system_settings USING btree (updated_at);


--
-- Name: alias_upstreams fk_model_aliases_upstreams; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alias_upstreams
    ADD CONSTRAINT fk_model_aliases_upstreams FOREIGN KEY (alias_id) REFERENCES public.model_aliases(id);


--
-- PostgreSQL database dump complete
--


