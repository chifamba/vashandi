import fs from "node:fs";
import https from "node:https";
import axios, { type AxiosInstance } from "axios";
import { logger } from "../middleware/logger.js";

interface OpenBrainClientOptions {
  baseURL: string;
  caCertPath: string;
  clientCertPath: string;
  clientKeyPath: string;
  enabled: boolean;
}

export class OpenBrainClient {
  private client: AxiosInstance | null = null;

  constructor(private options: OpenBrainClientOptions) {
    if (options.enabled) {
      this.initClient();
    }
  }

  private initClient() {
    try {
      const ca = fs.readFileSync(this.options.caCertPath);
      const cert = fs.readFileSync(this.options.clientCertPath);
      const key = fs.readFileSync(this.options.clientKeyPath);

      const agent = new https.Agent({
        ca,
        cert,
        key,
        // We ensure the hostname matches the certificate (ca, openbrain, etc.)
        // In local Docker, this is usually 'openbrain'
      });

      this.client = axios.create({
        baseURL: this.options.baseURL,
        httpsAgent: agent,
        headers: {
          "Content-Type": "application/json",
          // We can still use a bearer token as a secondary layer if needed
        },
      });

      logger.info({ baseURL: this.options.baseURL }, "OpenBrain mTLS client initialized");
    } catch (err) {
      logger.error({ err }, "Failed to initialize OpenBrain mTLS client");
      this.client = null;
    }
  }

  async createNamespace(namespaceId: string, companyId: string) {
    if (!this.client) return;
    try {
      await this.client.post("/internal/v1/namespaces", {
        namespaceId,
        companyId,
      });
      logger.info({ namespaceId, companyId }, "Namespace registered in OpenBrain");
    } catch (err) {
      logger.error({ err, namespaceId }, "Failed to register namespace in OpenBrain");
    }
  }

  async archiveNamespace(namespaceId: string) {
    if (!this.client) return;
    try {
      await this.client.delete(`/internal/v1/namespaces/${namespaceId}`);
      logger.info({ namespaceId }, "Namespace archived in OpenBrain");
    } catch (err) {
      logger.error({ err, namespaceId }, "Failed to archive namespace in OpenBrain");
    }
  }

  async deleteNamespace(namespaceId: string) {
    if (!this.client) return;
    try {
      // In OpenBrain internal API, DELETE handles both archiving and deletion (data is soft-deleted by status)
      await this.client.delete(`/internal/v1/namespaces/${namespaceId}`);
      logger.info({ namespaceId }, "Namespace deleted in OpenBrain");
    } catch (err) {
      logger.error({ err, namespaceId }, "Failed to delete namespace in OpenBrain");
    }
  }

  async registerAgent(namespaceId: string, agentId: string, agentName: string) {
    if (!this.client) return;
    try {
      await this.client.post(`/internal/v1/namespaces/${namespaceId}/agents`, {
        agentId,
        agentName,
      });
      logger.info({ namespaceId, agentId }, "Agent registered in OpenBrain");
    } catch (err) {
      logger.error({ err, agentId }, "Failed to register agent in OpenBrain");
    }
  }

  async deregisterAgent(namespaceId: string, agentId: string) {
    if (!this.client) return;
    try {
      await this.client.delete(`/internal/v1/namespaces/${namespaceId}/agents/${agentId}`);
      logger.info({ namespaceId, agentId }, "Agent deregistered in OpenBrain");
    } catch (err) {
      logger.error({ err, agentId }, "Failed to deregister agent in OpenBrain");
    }
  }

  async compileContext(namespaceId: string, agentId: string, intent: string, query?: string) {
    if (!this.client) return null;
    try {
      const response = await this.client.post("/api/v1/context/compile", {
        namespaceId,
        agentId,
        intent,
        query,
      });
      return response.data;
    } catch (err) {
      logger.error({ err, agentId }, "Failed to compile semantic context from OpenBrain");
      return null;
    }
  }

  async createMemory(namespaceId: string, payload: {
    entityType: string;
    text: string;
    title?: string;
    metadata?: Record<string, any>;
    tier?: number;
  }) {
    if (!this.client) return;
    try {
      await this.client.post("/api/v1/memories", {
        namespaceId,
        ...payload,
      });
      logger.info({ namespaceId, entityType: payload.entityType }, "Memory created in OpenBrain");
    } catch (err) {
      logger.error({ err, namespaceId }, "Failed to create memory in OpenBrain");
    }
  }
}

// Singleton instance configured from environment
export const openBrainClient = new OpenBrainClient({
  baseURL: process.env.OPENBRAIN_REST_URL || "https://openbrain:3101",
  caCertPath: process.env.OPENBRAIN_CA_CERT || "/certs/root_ca.crt",
  clientCertPath: process.env.OPENBRAIN_CLIENT_CERT || "/certs/vashandi.crt",
  clientKeyPath: process.env.OPENBRAIN_CLIENT_KEY || "/certs/vashandi.key",
  enabled: process.env.OPENBRAIN_MTLS_ENABLED === "true",
});
