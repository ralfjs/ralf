try { foo(); } finally { return 1; } // expect-error: no-unsafe-finally
