import { describe, expect, it } from "vitest";
import {
  isRememberableCompanyPath,
  getRememberedPathOwnerCompanyId,
  sanitizeRememberedPathForCompany,
} from "./company-page-memory";

describe("company-page-memory", () => {
  describe("isRememberableCompanyPath", () => {
    it("returns true for empty or root paths", () => {
      expect(isRememberableCompanyPath("")).toBe(true);
      expect(isRememberableCompanyPath("/")).toBe(true);
    });

    it("returns false for global segments", () => {
      expect(isRememberableCompanyPath("/auth")).toBe(false);
      expect(isRememberableCompanyPath("/invite/123")).toBe(false);
      expect(isRememberableCompanyPath("/board-claim")).toBe(false);
      expect(isRememberableCompanyPath("/cli-auth")).toBe(false);
      expect(isRememberableCompanyPath("/docs/api")).toBe(false);
    });

    it("returns true for normal board segments", () => {
      expect(isRememberableCompanyPath("/dashboard")).toBe(true);
      expect(isRememberableCompanyPath("/issues")).toBe(true);
      expect(isRememberableCompanyPath("/issues/PAP-123")).toBe(true);
      expect(isRememberableCompanyPath("/PAP/dashboard")).toBe(true); // segment is PAP
    });

    it("ignores query parameters", () => {
      expect(isRememberableCompanyPath("/auth?foo=bar")).toBe(false);
      expect(isRememberableCompanyPath("/dashboard?foo=bar")).toBe(true);
    });
  });

  describe("getRememberedPathOwnerCompanyId", () => {
    const companies = [
      { id: "c1", issuePrefix: "PAP" },
      { id: "c2", issuePrefix: "XYZ" },
    ];

    it("returns fallbackCompanyId if no route company prefix is found", () => {
      expect(
        getRememberedPathOwnerCompanyId({
          companies,
          pathname: "/dashboard", // No prefix here, just board route
          fallbackCompanyId: "fallback-id",
        })
      ).toBe("fallback-id");
    });

    it("returns the company id if route prefix matches a company issuePrefix", () => {
      expect(
        getRememberedPathOwnerCompanyId({
          companies,
          pathname: "/PAP/dashboard",
          fallbackCompanyId: "fallback-id",
        })
      ).toBe("c1");

      expect(
        getRememberedPathOwnerCompanyId({
          companies,
          pathname: "/xyz/issues", // Case insensitive match
          fallbackCompanyId: "fallback-id",
        })
      ).toBe("c2");
    });

    it("returns null if route prefix exists but does not match any company", () => {
      expect(
        getRememberedPathOwnerCompanyId({
          companies,
          pathname: "/ABC/dashboard",
          fallbackCompanyId: "fallback-id",
        })
      ).toBeNull();
    });
  });

  describe("sanitizeRememberedPathForCompany", () => {
    it("returns /dashboard for null or undefined path", () => {
      expect(sanitizeRememberedPathForCompany({ path: null, companyPrefix: "PAP" })).toBe("/dashboard");
      expect(sanitizeRememberedPathForCompany({ path: undefined, companyPrefix: "PAP" })).toBe("/dashboard");
    });

    it("returns /dashboard for unrememberable paths", () => {
      expect(sanitizeRememberedPathForCompany({ path: "/auth", companyPrefix: "PAP" })).toBe("/dashboard");
      expect(sanitizeRememberedPathForCompany({ path: "/invite/123", companyPrefix: "PAP" })).toBe("/dashboard");
    });

    it("returns relative path for valid rememberable paths", () => {
      expect(sanitizeRememberedPathForCompany({ path: "/dashboard", companyPrefix: "PAP" })).toBe("/dashboard");
      expect(sanitizeRememberedPathForCompany({ path: "/PAP/dashboard", companyPrefix: "PAP" })).toBe("/dashboard");
      expect(sanitizeRememberedPathForCompany({ path: "/PAP/issues", companyPrefix: "PAP" })).toBe("/issues");
    });

    it("handles cross-company issue paths correctly", () => {
      // Different company prefix in the entity id -> fallback to /dashboard
      expect(
        sanitizeRememberedPathForCompany({ path: "/XYZ/issues/XYZ-123", companyPrefix: "PAP" })
      ).toBe("/dashboard");
      expect(
        sanitizeRememberedPathForCompany({ path: "/issues/XYZ-123", companyPrefix: "PAP" })
      ).toBe("/dashboard");

      // Same company prefix -> keep the path
      expect(
        sanitizeRememberedPathForCompany({ path: "/PAP/issues/PAP-123", companyPrefix: "PAP" })
      ).toBe("/issues/PAP-123");
      expect(
        sanitizeRememberedPathForCompany({ path: "/issues/PAP-123", companyPrefix: "PAP" })
      ).toBe("/issues/PAP-123");

      // Case insensitivity
      expect(
        sanitizeRememberedPathForCompany({ path: "/issues/pap-123", companyPrefix: "PAP" })
      ).toBe("/issues/pap-123");
    });
  });
});
