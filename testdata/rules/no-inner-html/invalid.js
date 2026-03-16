element.innerHTML = "<b>bold</b>"; // expect-error: no-inner-html
el.innerHTML = value; // expect-error: no-inner-html
document.body.innerHTML = ""; // expect-error: no-inner-html
const x = 1;
