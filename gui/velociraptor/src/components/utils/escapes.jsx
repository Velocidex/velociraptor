export function isString(value) {
  return Object.prototype.toString.call(value) === '[object String]';
}


// Escapes the value from backticks and returns a VQL identifier.
export function escapeIdentifer(value) {
    if(isString(value)) {
        return "`" + value.replace(/`/g, "") + "`";
    }
    return "";
}

export function escapeMultiLine(value) {
    if(isString(value)) {
        return "'''" + value.replace(/'''/g, "") + "'''";
    }
    return "''";
}

export function quoteString(value) {
    if(isString(value)) {
        return JSON.stringify(value);
    }
    return "";
}
