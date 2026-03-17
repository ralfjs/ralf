var p = new Promise(async (resolve, reject) => { resolve(1); }); // expect-error: no-async-promise-executor
