
/* Filesystems use a complex system for representing path
 * components. Velociraptor abstract this by treating paths as an
 * array of components.

 Components are joined using the path separator "/" or "\" character
 (these are treated the same).

 It is allowed to have path separated within the component itself -
 this simply means that the component will be escaped with quotes.

 All quotes within a quotted component are doubled up.

 For example:

 \\.\C:\Windows\System32  means ["\\.\C:", "Windows", "System32"]

 would be represented as
 "\\.\C:"\Windows\System32

*/

// Join a component to an existing path - escapes quotes if required.
const PathJoin = function(path, component) {
    // If we need to escape the component we do so here.
    if (component.includes("/") ||
        component[0] === "\"" ||
        component.includes("\\")) {
        return path + '/"' + component.replace(/"/g, '""') + '"';
    }
    // Otherwise it is safe to join it.
    return path + '/' + component;
};

// Parse the path and extract the first component from it.
// Returns an object {next_path: string, component: string}
const ConsumeComponent = function(path) {
    if (path.length === 0) {
        return {next_path: "", component: ""};
    }

    if (path[0] === '/' || path[0] === '\\') {
        return {next_path: path.substr(1, path.length), component: ""};
    }

    if (path[0] === '"') {
        var result = "";
        for (var i=1; i<path.length; i++) {
            if (path[i] === '"') {
                if (i >= path.length-1) {
                    return {next_path: "", component: result};
                }

                var next_char = path[i+1];
                if (next_char === '"') { // Double quoted quote
                    result += next_char;
                    i += 1;

                } else if(next_char === '/' || next_char === '\\') {
                    return {next_path: path.substr(i+1, path.length),
                            component: result};

                } else {
                    // Should never happen, " followed by *
                    result += next_char;
                }

            } else {
                result += path[i];
            }
        }

        // If we get here it is unterminated (e.g. '"foo<EOF>')
        return {next_path: "", component: result};

    } else {
        for (var j = 0; j < path.length; j++) {
            if (path[j] === '/' || path[i] === '\\') {
                return {next_path: path.substr(j, path.length),
                        component: path.substr(0, j)};
            }
        }
    }

    return {next_path: "", component: path};
};


// Split a path into a list of components
export const SplitPathComponents = function(path) {
    var components = [];

    while (path !== "") {
        var item = ConsumeComponent(path);
        if (item.component !== "") {
            components.push(item.component);
        }
        path = item.next_path;
    }

    return components;
};

// Joins components into a path string.
export const Join = function(components) {
    var result = "";
    for (var i = 0; i < components.length; i++) {
        result = PathJoin(result, components[i]);
    }

    return result;
};


// Work around encoding bugs in react router.
export const EncodePathInURL = function(path) {
    path = path.replace(/-/g, "%2d");
    return encodeURI(path).replace(/%/g, "-");
};


export const DecodePathInURL = function(path) {
    return decodeURI(path.replace(/-/g, "%"));
};
