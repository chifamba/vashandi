-- Enable extensions in public schema
CREATE EXTENSION IF NOT EXISTS "uuid-ossp" SCHEMA public;
CREATE EXTENSION IF NOT EXISTS "vector" SCHEMA public;

-- Create Vashandi user and schema
CREATE USER vashandi WITH PASSWORD 'vashandi_password';
CREATE SCHEMA IF NOT EXISTS vashandi AUTHORIZATION vashandi;
GRANT ALL PRIVILEGES ON SCHEMA vashandi TO vashandi;

-- Create OpenBrain user and schema
CREATE USER openbrain WITH PASSWORD 'openbrain_password';
CREATE SCHEMA IF NOT EXISTS openbrain AUTHORIZATION openbrain;
GRANT ALL PRIVILEGES ON SCHEMA openbrain TO openbrain;

-- Ensure users can see the public schema where extensions live
GRANT USAGE ON SCHEMA public TO vashandi;
GRANT USAGE ON SCHEMA public TO openbrain;

-- Add public to search path for these users
ALTER USER vashandi SET search_path = vashandi, public;
ALTER USER openbrain SET search_path = openbrain, public;

-- Grant explicit access to vector type functions
GRANT ALL ON SCHEMA public TO openbrain;
