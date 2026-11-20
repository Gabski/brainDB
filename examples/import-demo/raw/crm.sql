--
-- PostgreSQL database dump
--
-- Dumped from database version 16.4
-- Fictional demo data for brainDB (Pellucid Coffee Systems CRM)

SET statement_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);

CREATE TABLE public.customers (
    id integer NOT NULL,
    name text NOT NULL,
    segment text NOT NULL,
    status text NOT NULL,
    city text,
    owner text,
    customer_since date,
    legal_name text,
    website text,
    vat_no text,
    CONSTRAINT customers_pkey PRIMARY KEY (id)
);

CREATE TABLE public.contacts (
    id integer PRIMARY KEY,
    customer_id integer NOT NULL REFERENCES public.customers(id),
    full_name text NOT NULL,
    role text,
    email text,
    phone text,
    is_primary boolean DEFAULT false
);

CREATE TABLE public.orders (
    id integer NOT NULL,
    customer_id integer NOT NULL,
    contact_id integer,
    sku text NOT NULL,
    quantity integer NOT NULL,
    total numeric(12,2) NOT NULL,
    currency character(3) DEFAULT 'EUR'::bpchar NOT NULL,
    status text NOT NULL,
    created_at date NOT NULL,
    CONSTRAINT orders_pkey PRIMARY KEY (id),
    CONSTRAINT orders_customer_fk FOREIGN KEY (customer_id) REFERENCES public.customers(id),
    CONSTRAINT orders_contact_fk FOREIGN KEY (contact_id) REFERENCES public.contacts(id)
);

--
-- Data for Name: customers; Type: TABLE DATA
--

COPY public.customers (id, name, segment, status, city, owner, customer_since, legal_name, website, vat_no) FROM stdin;
1	Harbor Lane Hotels	enterprise	active	Oslo	Mira Sandvik	2021-03-14	Harbor Lane Hotels AS	https://www.harbor.example	NO 910 007 319 MVA
2	Copperleaf Cafés	mid-market	active	Bergen	Tobias Lund	2022-06-01	Copperleaf Cafés AS	https://www.copperleaf.example	NO 910 014 625 MVA
3	Northgate Offices	enterprise	active	Stockholm	Mira Sandvik	2020-11-20	Northgate Offices AB	https://www.northgate.example	\N
4	Beanstalk Kiosk Co	smb	active	Gothenburg	Elif Kaya	2023-02-08	Beanstalk Kiosk Co AB	https://www.beanstalk.example	\N
5	Fjordline Ferries	enterprise	active	Kristiansand	Tobias Lund	2019-09-30	Fjordline Ferries AS	https://www.fjordline.example	NO 910 036 556 MVA
6	Little Ember Bakery	smb	churned	Aarhus	Elif Kaya	2022-01-17	Little Ember Bakery ApS	https://www.little.example	\N
7	Saltmarsh Coworking	mid-market	active	Copenhagen	Elif Kaya	2023-05-22	Saltmarsh Coworking ApS	https://www.saltmarsh.example	\N
8	Tidewater University	enterprise	active	Uppsala	Mira Sandvik	2021-08-09	Tidewater University AB	https://www.tidewater.example	\N
9	Granite & Grain Diner	smb	trial	Trondheim	Tobias Lund	2026-10-02	Granite & Grain Diner AS	https://www.granite.example	NO 910 065 793 MVA
10	Pinecrest Clinics	mid-market	active	Tampere	Elif Kaya	2024-04-11	Pinecrest Clinics Oy	https://www.pinecrest.example	\N
11	Larkfield Roastery	smb	active	Malmö	Tobias Lund	2023-09-19	Larkfield Roastery AB	https://www.larkfield.example	\N
12	Meridian Airports Lounge	enterprise	active	Helsinki	Mira Sandvik	2020-02-03	Meridian Airports Lounge Oy	https://www.meridian.example	\N
13	Oxbow Bookshop Café	smb	churned	Odense	Elif Kaya	2021-12-12	Oxbow Bookshop Café ApS	https://www.oxbow.example	\N
14	Westbrook Fitness	mid-market	trial	Stavanger	Tobias Lund	2026-09-15	Westbrook Fitness AS	https://www.westbrook.example	NO 910 102 346 MVA
15	Silverbirch Conference Centre	mid-market	active	Turku	Mira Sandvik	2022-10-27	Silverbirch Conference Centre Oy	https://www.silverbirch.example	\N
\.

--
-- Data for Name: contacts (multi-row INSERT)
--

INSERT INTO public.contacts (id, customer_id, full_name, role, email, phone, is_primary) VALUES
    (1, 1, 'Anna Holm', 'procurement lead', 'anna.holm@harbor.example', '+47 22 10 47 29', TRUE),
    (2, 1, 'Jonas Berg', 'operations director', 'jonas.berg@harbor.example', '+47 22 20 94 58', FALSE),
    (3, 2, 'Sara Dahl', 'operations director', 'sara.dahl@copperleaf.example', '+47 22 31 41 87', TRUE),
    (4, 2, 'Mikkel Strand', 'facilities manager', 'mikkel.strand@copperleaf.example', '+47 22 41 89 16', FALSE),
    (5, 3, 'Ida Nyberg', 'facilities manager', 'ida.nyberg@northgate.example', '+46 8 225 23 64', TRUE),
    (6, 3, 'Lars Lind', 'head barista', 'lars.lind@northgate.example', '+46 8 226 28 37', FALSE),
    (7, 4, 'Freja Moe', 'head barista', 'freja.moe@beanstalk.example', '+46 8 227 33 10', TRUE),
    (8, 5, 'Oskar Aas', 'operations director', 'oskar.aas@fjordline.example', '+47 22 83 78 32', TRUE),
    (9, 5, 'Nora Vik', 'facilities manager', 'nora.vik@fjordline.example', '+47 22 94 25 61', FALSE),
    (10, 6, 'Emil Sund', 'facilities manager', 'emil.sund@little.example', '+45 23 04 72 90', TRUE),
    (11, 7, 'Hanna Eklund', 'procurement lead', 'hanna.eklund@saltmarsh.example', '+45 23 15 20 19', TRUE),
    (12, 7, 'Viktor Hagen', 'operations director', 'viktor.hagen@saltmarsh.example', '+45 23 25 67 48', FALSE),
    (13, 8, 'Maja Brekke', 'operations director', 'maja.brekke@tidewater.example', '+46 8 233 61 47', TRUE),
    (14, 8, 'Axel Falk', 'facilities manager', 'axel.falk@tidewater.example', '+46 8 234 66 20', FALSE),
    (15, 9, 'Linnea Rask', 'facilities manager', 'linnea.rask@granite.example', '+47 23 57 09 35', TRUE),
    (16, 10, 'Henrik Wiik', 'procurement lead', 'henrik.wiik@pinecrest.example', '+358 9 2367 566', TRUE),
    (17, 10, 'Sofie Lie', 'operations director', 'sofie.lie@pinecrest.example', '+358 9 2378 039', FALSE),
    (18, 11, 'Rasmus Solheim', 'operations director', 'rasmus.solheim@larkfield.example', '+46 8 238 85 12', TRUE),
    (19, 11, 'Elin Kvist', 'facilities manager', 'elin.kvist@larkfield.example', '+46 8 239 89 85', FALSE),
    (20, 12, 'Mats Tangen', 'facilities manager', 'mats.tangen@meridian.example', '+358 9 2409 458', TRUE),
    (21, 12, 'Karin Ruud', 'head barista', 'karin.ruud@meridian.example', '+358 9 2419 930', FALSE),
    (22, 13, 'Petter Hovland', 'head barista', 'petter.hovland@oxbow.example', '+45 24 30 40 38', TRUE),
    (23, 14, 'Astrid Engen', 'operations director', 'astrid.engen@westbrook.example', '+47 24 40 87 67', TRUE),
    (24, 15, 'Nils Nesse', 'finance controller', 'nils.nesse@silverbirch.example', '+358 9 2451 349', TRUE),
    (25, 15, 'Tuva Borg', 'procurement lead', 'tuva.borg@silverbirch.example', '+358 9 2461 822', FALSE);

--
-- Data for Name: orders; Type: TABLE DATA
--

COPY public.orders (id, customer_id, contact_id, sku, quantity, total, currency, status, created_at) FROM stdin;
1001	8	13	GR-64	12	17400.00	EUR	shipped	2026-06-12
1002	15	24	SV-CARE	1	1200.00	EUR	invoiced	2026-11-23
1003	7	11	PX-300	1	10900.00	EUR	invoiced	2026-05-07
1004	14	23	GR-83	12	28200.00	EUR	invoiced	2026-10-18
1005	6	10	SV-PRIO	1	2600.00	EUR	invoiced	2026-04-02
1006	13	22	AX-1	1	5400.00	EUR	invoiced	2026-09-13
1007	5	8	WF-10	12	1680.00	EUR	shipped	2026-03-24
1008	12	20	PX-200	5	39500.00	EUR	open	2026-08-08
1009	4	7	GR-64	4	5800.00	EUR	shipped	2026-02-19
1010	11	18	SV-CARE	1	1200.00	EUR	invoiced	2026-07-03
1011	3	5	PX-300	2	21800.00	EUR	invoiced	2026-01-14
1012	10	16	GR-83	12	28200.00	EUR	invoiced	2026-06-25
1013	2	3	SV-PRIO	1	2600.00	EUR	cancelled	2026-11-09
1014	9	15	AX-1	1	5400.00	EUR	open	2026-05-20
1015	1	1	WF-10	4	560.00	EUR	shipped	2026-10-04
1016	8	13	PX-200	2	15800.00	EUR	open	2026-04-15
1017	15	24	GR-64	12	17400.00	EUR	shipped	2026-09-26
1018	7	11	SV-CARE	1	1200.00	EUR	invoiced	2026-03-10
1019	14	23	PX-300	1	10900.00	EUR	invoiced	2026-08-21
1020	7	11	GR-83	6	14100.00	EUR	invoiced	2026-02-05
1021	14	23	SV-PRIO	1	2600.00	EUR	cancelled	2026-07-16
1022	5	8	AX-1	2	10800.00	EUR	open	2026-01-27
1023	12	20	WF-10	2	280.00	EUR	shipped	2026-06-11
1024	4	7	PX-200	1	7900.00	EUR	open	2026-11-22
1025	11	18	GR-64	2	2900.00	EUR	shipped	2026-05-06
1026	3	5	SV-CARE	1	1200.00	EUR	invoiced	2026-10-17
1027	10	16	PX-300	1	10900.00	EUR	invoiced	2026-04-01
1028	2	3	GR-83	12	28200.00	EUR	invoiced	2026-09-12
1029	9	15	SV-PRIO	1	2600.00	EUR	cancelled	2026-03-23
1030	1	1	AX-1	3	16200.00	EUR	open	2026-08-07
\.

-- late orders added by the web shop sync
INSERT INTO public.orders VALUES
    (1031, 8, 13, 'WF-10', 4, 560.00, 'EUR', 'shipped', '2026-02-18'),
    (1032, 15, 24, 'PX-200', 1, 7900.00, 'EUR', 'open', '2026-07-02'),
    (1033, 7, 11, 'GR-64', 2, 2900.00, 'EUR', 'shipped', '2026-01-13'),
    (1034, 14, 23, 'SV-CARE', 1, 1200.00, 'EUR', 'open', '2026-11-04'),
    (1035, 7, 11, 'PX-300', 1, 10900.00, 'EUR', 'open', '2026-11-05'),
    (1036, 14, 23, 'GR-83', 2, 4700.00, 'EUR', 'open', '2026-11-06'),
    (1037, 5, 8, 'SV-PRIO', 1, 2600.00, 'EUR', 'open', '2026-11-07'),
    (1038, 12, 20, 'AX-1', 2, 10800.00, 'EUR', 'open', '2026-11-08'),
    (1039, 4, 7, 'WF-10', 2, 280.00, 'EUR', 'open', '2026-11-09'),
    (1040, 11, 18, 'PX-200', 1, 7900.00, 'EUR', 'open', '2026-11-10');

CREATE INDEX orders_customer_idx ON public.orders USING btree (customer_id);

--
-- PostgreSQL database dump complete
--
