import React from "react";

function Button({ label, onClick, disabled }) {
  return (
    <button className="btn" onClick={onClick} disabled={disabled}>
      {label}
    </button>
  );
}

function App() {
  return (
    <div>
      <h1>Hello</h1>
      <Button label="Click me" onClick={() => alert("clicked")} />
    </div>
  );
}

export default App;
