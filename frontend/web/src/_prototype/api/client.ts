// Prototype-local ApiError mirror. Kept separate from main @/api/client so the
// prototype has zero hard dependency on the real auth/openapi-fetch layer —
// mocks under _prototype/api/* never throw this and the existing instanceof
// guards in Aidana's code continue to compile and behave the same way.
export class ApiError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string) {
    super(code);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}
