import { describe, it, expect } from "vitest";
import {
  getPortableFileText,
  getPortableFileContentType,
  getPortableFileDataUrl,
  isPortableImageFile,
} from "./portable-files";
import type { CompanyPortabilityFileEntry } from "@paperclipai/shared";

describe("portable-files", () => {
  describe("getPortableFileText", () => {
    it("returns string if entry is a string", () => {
      expect(getPortableFileText("test text")).toBe("test text");
    });

    it("returns null if entry is an object", () => {
      expect(getPortableFileText({ data: "data" })).toBeNull();
    });

    it("returns null if entry is null or undefined", () => {
      expect(getPortableFileText(null)).toBeNull();
      expect(getPortableFileText(undefined)).toBeNull();
    });
  });

  describe("getPortableFileContentType", () => {
    it("returns contentType from object entry if present", () => {
      const entry: CompanyPortabilityFileEntry = { data: "base64", contentType: "image/png" };
      expect(getPortableFileContentType("file.jpg", entry)).toBe("image/png");
    });

    it("returns contentType based on file extension if not present in object", () => {
      const entry: CompanyPortabilityFileEntry = { data: "base64" };
      expect(getPortableFileContentType("file.png", entry)).toBe("image/png");
    });

    it("returns contentType based on file extension if entry is a string", () => {
      expect(getPortableFileContentType("file.svg", "text")).toBe("image/svg+xml");
    });

    it("returns contentType based on file extension if entry is null", () => {
      expect(getPortableFileContentType("file.webp", null)).toBe("image/webp");
    });

    it("returns null for unknown extensions", () => {
      expect(getPortableFileContentType("file.unknown", null)).toBeNull();
    });

    it("returns null for file with no extension", () => {
      expect(getPortableFileContentType("file", null)).toBeNull();
    });

    it("is case insensitive for file extensions", () => {
      expect(getPortableFileContentType("FILE.PNG", null)).toBe("image/png");
    });
  });

  describe("getPortableFileDataUrl", () => {
    it("returns null if entry is a string", () => {
      expect(getPortableFileDataUrl("file.png", "string entry")).toBeNull();
    });

    it("returns null if entry is null or undefined", () => {
      expect(getPortableFileDataUrl("file.png", null)).toBeNull();
      expect(getPortableFileDataUrl("file.png", undefined)).toBeNull();
    });

    it("returns data url with default content type if not inferrable", () => {
      const entry: CompanyPortabilityFileEntry = { data: "SGVsbG8=" };
      expect(getPortableFileDataUrl("file.unknown", entry)).toBe("data:application/octet-stream;base64,SGVsbG8=");
    });

    it("returns data url with inferred content type", () => {
      const entry: CompanyPortabilityFileEntry = { data: "SGVsbG8=" };
      expect(getPortableFileDataUrl("file.png", entry)).toBe("data:image/png;base64,SGVsbG8=");
    });

    it("returns data url with provided content type", () => {
      const entry: CompanyPortabilityFileEntry = { data: "SGVsbG8=", contentType: "image/svg+xml" };
      expect(getPortableFileDataUrl("file.png", entry)).toBe("data:image/svg+xml;base64,SGVsbG8=");
    });
  });

  describe("isPortableImageFile", () => {
    it("returns true for valid image file extensions", () => {
      expect(isPortableImageFile("image.png", null)).toBe(true);
      expect(isPortableImageFile("image.jpg", null)).toBe(true);
      expect(isPortableImageFile("image.jpeg", null)).toBe(true);
      expect(isPortableImageFile("image.gif", null)).toBe(true);
      expect(isPortableImageFile("image.svg", null)).toBe(true);
      expect(isPortableImageFile("image.webp", null)).toBe(true);
    });

    it("returns false for non-image file extensions", () => {
      expect(isPortableImageFile("document.pdf", null)).toBe(false);
      expect(isPortableImageFile("text.txt", null)).toBe(false);
    });

    it("returns true if entry has image content type", () => {
      const entry: CompanyPortabilityFileEntry = { data: "base64", contentType: "image/bmp" };
      expect(isPortableImageFile("file.unknown", entry)).toBe(true);
    });

    it("returns false if entry has non-image content type", () => {
      const entry: CompanyPortabilityFileEntry = { data: "base64", contentType: "text/plain" };
      expect(isPortableImageFile("image.png", entry)).toBe(false);
    });

    it("returns false if no extension and no entry", () => {
      expect(isPortableImageFile("file", null)).toBe(false);
    });
  });
});
