import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { extractMediaResultFromToolResult } from "./mediaResult";

describe("extractMediaResultFromToolResult", () => {
  it("extracts generated image URLs from tool output", () => {
    assert.deepEqual(
      extractMediaResultFromToolResult(
        "generate_image",
        "[generate_image] https://cdn.example.com/out.png?Expires=1&Signature=x",
      ),
      {
        type: "image",
        url: "https://cdn.example.com/out.png?Expires=1&Signature=x",
      },
    );
  });

  it("converts absolute generated image paths to api file URLs", () => {
    assert.deepEqual(
      extractMediaResultFromToolResult(
        "generate_image",
        "[generate_image] /tmp/saker-media/out.webp",
      ),
      {
        type: "image",
        url: "/api/files/tmp/saker-media/out.webp",
      },
    );
  });

  it("converts .saker/media relative paths to api file URLs", () => {
    assert.deepEqual(
      extractMediaResultFromToolResult(
        "generate_image",
        "[generate_image] .saker/media/saker-media-abc123.png",
      ),
      {
        type: "image",
        url: "/api/files/.saker/media/saker-media-abc123.png",
      },
    );
  });

  it("ignores non-media tools", () => {
    assert.equal(
      extractMediaResultFromToolResult(
        "bash",
        "https://cdn.example.com/out.png",
      ),
      null,
    );
  });
});
