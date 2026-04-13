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

      logger.info("OpenBrain mTLS client initialized", { baseURL: this.options.baseURL });
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
      logger.info("Namespace registered in OpenBrain", { namespaceId, companyId });
    } catch (err) {
      logger.error({ err, namespaceId }, "Failed to register namespace in OpenBrain");
    }
  }

  async registerAgent(namespaceId: string, agentId: string, name: string) {
    if (!this.client) return;
    try {
      await this.client.post(`/internal/v1/namespaces/${namespaceId}/agents`, {
        agentId,
        name,
      });
      logger.info("Agent registered in OpenBrain", { namespaceId, agentId });
    } catch (err) {
      logger.error({ err, agentId }, "Failed to register agent in OpenBrain");
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
