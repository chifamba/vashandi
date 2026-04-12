import { VashandiActor } from "@paperclipai/shared";

declare global {
  namespace Express {
    interface Request {
      actor?: VashandiActor;
      plugin?: any;
    }
  }
}
