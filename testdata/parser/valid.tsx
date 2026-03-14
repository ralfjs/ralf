import React from "react";

interface Props {
  title: string;
  count: number;
  onIncrement: () => void;
}

function Counter({ title, count, onIncrement }: Props): JSX.Element {
  return (
    <div className="counter">
      <h2>{title}</h2>
      <span>{count}</span>
      <button onClick={onIncrement}>+1</button>
    </div>
  );
}

export default Counter;
