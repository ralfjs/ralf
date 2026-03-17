try { doSomething(); } catch (e) { throw new Error(e.message); }
try { doSomething(); } catch (e) { console.log(e); throw e; }
