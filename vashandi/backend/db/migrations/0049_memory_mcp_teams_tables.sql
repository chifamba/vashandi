-- Migration: Add memory service tables, MCP governance tables, and teams tables
-- These tables support Go backend features defined in backend/db/models/
--
-- Rollback:
--   DROP INDEX IF EXISTS "memory_operations_company_idx";
--   DROP INDEX IF EXISTS "memory_operations_binding_idx";
--   DROP INDEX IF EXISTS "memory_binding_targets_company_idx";
--   DROP INDEX IF EXISTS "memory_binding_targets_binding_idx";
--   DROP INDEX IF EXISTS "memory_binding_targets_target_idx";
--   DROP INDEX IF EXISTS "memory_bindings_company_idx";
--   DROP INDEX IF EXISTS "mcp_tool_definitions_company_idx";
--   DROP INDEX IF EXISTS "mcp_entitlement_profiles_company_idx";
--   DROP INDEX IF EXISTS "agent_mcp_entitlements_company_idx";
--   DROP INDEX IF EXISTS "agent_mcp_entitlements_agent_idx";
--   DROP INDEX IF EXISTS "agent_mcp_entitlements_profile_idx";
--   DROP INDEX IF EXISTS "teams_company_idx";
--   DROP INDEX IF EXISTS "team_memberships_company_team_idx";
--   DROP INDEX IF EXISTS "team_memberships_company_agent_idx";
--   DROP INDEX IF EXISTS "team_budgets_company_idx";
--   DROP INDEX IF EXISTS "team_budgets_team_idx";
--   DROP TABLE IF EXISTS "memory_operations";
--   DROP TABLE IF EXISTS "memory_binding_targets";
--   DROP TABLE IF EXISTS "memory_bindings";
--   DROP TABLE IF EXISTS "agent_mcp_entitlements";
--   DROP TABLE IF EXISTS "mcp_entitlement_profiles";
--   DROP TABLE IF EXISTS "mcp_tool_definitions";
--   DROP TABLE IF EXISTS "team_budgets";
--   DROP TABLE IF EXISTS "team_memberships";
--   DROP TABLE IF EXISTS "teams";

-- ============================================================================
-- MEMORY SERVICE TABLES (Task 17)
-- These tables support the memory service integration with OpenBrain
-- See: doc/plans/2026-03-17-memory-service-surface-api.md
-- ============================================================================

CREATE TABLE "memory_bindings" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"company_id" uuid NOT NULL,
	"key" text NOT NULL,
	"provider_plugin_id" text NOT NULL,
	"config" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"enabled" boolean DEFAULT true NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "memory_binding_targets" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"company_id" uuid NOT NULL,
	"binding_id" uuid NOT NULL,
	"target_type" varchar(50) NOT NULL,
	"target_id" uuid NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "memory_operations" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"company_id" uuid NOT NULL,
	"binding_id" uuid NOT NULL,
	"operation_type" varchar(50) NOT NULL,
	"scope" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"source_ref" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"usage" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"success" boolean NOT NULL,
	"error" text,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
ALTER TABLE "memory_bindings" ADD CONSTRAINT "memory_bindings_company_id_companies_id_fk" FOREIGN KEY ("company_id") REFERENCES "public"."companies"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "memory_binding_targets" ADD CONSTRAINT "memory_binding_targets_company_id_companies_id_fk" FOREIGN KEY ("company_id") REFERENCES "public"."companies"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "memory_binding_targets" ADD CONSTRAINT "memory_binding_targets_binding_id_memory_bindings_id_fk" FOREIGN KEY ("binding_id") REFERENCES "public"."memory_bindings"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "memory_operations" ADD CONSTRAINT "memory_operations_company_id_companies_id_fk" FOREIGN KEY ("company_id") REFERENCES "public"."companies"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "memory_operations" ADD CONSTRAINT "memory_operations_binding_id_memory_bindings_id_fk" FOREIGN KEY ("binding_id") REFERENCES "public"."memory_bindings"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "memory_bindings_company_idx" ON "memory_bindings" USING btree ("company_id");--> statement-breakpoint
CREATE INDEX "memory_binding_targets_company_idx" ON "memory_binding_targets" USING btree ("company_id");--> statement-breakpoint
CREATE INDEX "memory_binding_targets_binding_idx" ON "memory_binding_targets" USING btree ("binding_id");--> statement-breakpoint
CREATE INDEX "memory_binding_targets_target_idx" ON "memory_binding_targets" USING btree ("target_type","target_id");--> statement-breakpoint
CREATE INDEX "memory_operations_company_idx" ON "memory_operations" USING btree ("company_id");--> statement-breakpoint
CREATE INDEX "memory_operations_binding_idx" ON "memory_operations" USING btree ("binding_id");

-- ============================================================================
-- MCP GOVERNANCE TABLES (Task 18)
-- These tables support MCP tool definitions and agent entitlements
-- ============================================================================

--> statement-breakpoint
CREATE TABLE "mcp_tool_definitions" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"company_id" uuid NOT NULL,
	"name" text NOT NULL,
	"description" text,
	"schema_json" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"source" varchar(50) NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "mcp_entitlement_profiles" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"company_id" uuid NOT NULL,
	"name" text NOT NULL,
	"tool_ids" text[] DEFAULT '{}'::text[] NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "agent_mcp_entitlements" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"company_id" uuid NOT NULL,
	"agent_id" uuid NOT NULL,
	"profile_id" uuid NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
ALTER TABLE "mcp_tool_definitions" ADD CONSTRAINT "mcp_tool_definitions_company_id_companies_id_fk" FOREIGN KEY ("company_id") REFERENCES "public"."companies"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "mcp_entitlement_profiles" ADD CONSTRAINT "mcp_entitlement_profiles_company_id_companies_id_fk" FOREIGN KEY ("company_id") REFERENCES "public"."companies"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_mcp_entitlements" ADD CONSTRAINT "agent_mcp_entitlements_company_id_companies_id_fk" FOREIGN KEY ("company_id") REFERENCES "public"."companies"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_mcp_entitlements" ADD CONSTRAINT "agent_mcp_entitlements_agent_id_agents_id_fk" FOREIGN KEY ("agent_id") REFERENCES "public"."agents"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "agent_mcp_entitlements" ADD CONSTRAINT "agent_mcp_entitlements_profile_id_mcp_entitlement_profiles_id_fk" FOREIGN KEY ("profile_id") REFERENCES "public"."mcp_entitlement_profiles"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "mcp_tool_definitions_company_idx" ON "mcp_tool_definitions" USING btree ("company_id");--> statement-breakpoint
CREATE INDEX "mcp_entitlement_profiles_company_idx" ON "mcp_entitlement_profiles" USING btree ("company_id");--> statement-breakpoint
CREATE INDEX "agent_mcp_entitlements_company_idx" ON "agent_mcp_entitlements" USING btree ("company_id");--> statement-breakpoint
CREATE INDEX "agent_mcp_entitlements_agent_idx" ON "agent_mcp_entitlements" USING btree ("agent_id");--> statement-breakpoint
CREATE INDEX "agent_mcp_entitlements_profile_idx" ON "agent_mcp_entitlements" USING btree ("profile_id");

-- ============================================================================
-- TEAMS TABLES (Task 19)
-- These tables are Go-only backend features for agent team management
-- Note: Not exposed in Node.js Drizzle schema - backend-specific feature
-- ============================================================================

--> statement-breakpoint
CREATE TABLE "teams" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"company_id" uuid NOT NULL,
	"name" text NOT NULL,
	"description" text,
	"lead_agent_id" uuid,
	"status" varchar(50) DEFAULT 'active' NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "team_memberships" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"company_id" uuid NOT NULL,
	"team_id" uuid NOT NULL,
	"agent_id" uuid NOT NULL,
	"role" varchar(50) NOT NULL,
	"joined_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "team_budgets" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"company_id" uuid NOT NULL,
	"team_id" uuid NOT NULL,
	"limit" numeric NOT NULL,
	"period" varchar(50) NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
ALTER TABLE "teams" ADD CONSTRAINT "teams_company_id_companies_id_fk" FOREIGN KEY ("company_id") REFERENCES "public"."companies"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "teams" ADD CONSTRAINT "teams_lead_agent_id_agents_id_fk" FOREIGN KEY ("lead_agent_id") REFERENCES "public"."agents"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "team_memberships" ADD CONSTRAINT "team_memberships_company_id_companies_id_fk" FOREIGN KEY ("company_id") REFERENCES "public"."companies"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "team_memberships" ADD CONSTRAINT "team_memberships_team_id_teams_id_fk" FOREIGN KEY ("team_id") REFERENCES "public"."teams"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "team_memberships" ADD CONSTRAINT "team_memberships_agent_id_agents_id_fk" FOREIGN KEY ("agent_id") REFERENCES "public"."agents"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "team_budgets" ADD CONSTRAINT "team_budgets_company_id_companies_id_fk" FOREIGN KEY ("company_id") REFERENCES "public"."companies"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "team_budgets" ADD CONSTRAINT "team_budgets_team_id_teams_id_fk" FOREIGN KEY ("team_id") REFERENCES "public"."teams"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "teams_company_idx" ON "teams" USING btree ("company_id");--> statement-breakpoint
CREATE INDEX "team_memberships_company_team_idx" ON "team_memberships" USING btree ("company_id","team_id");--> statement-breakpoint
CREATE INDEX "team_memberships_company_agent_idx" ON "team_memberships" USING btree ("company_id","agent_id");--> statement-breakpoint
CREATE INDEX "team_budgets_company_idx" ON "team_budgets" USING btree ("company_id");--> statement-breakpoint
CREATE INDEX "team_budgets_team_idx" ON "team_budgets" USING btree ("team_id");
