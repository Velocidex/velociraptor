// Safe use of html-react-parser combined it with dompurify to only
// allow proper tag and value.

// NOTE: This is also done in the go server using bluemonday but this
// is a second layer of defence in case something slips in we do it
// again in javascript.
import parse from 'html-react-parser';
import DOMPurify from 'dompurify';

export function sanitize(html) {
    let clean = DOMPurify.sanitize(html, {
        USE_PROFILES: { html: true },

        // Line up with NewBlueMondayPolicy() in reporting/gui.go
        CUSTOM_ELEMENT_HANDLING: {
            tagNameCheck: /^(notebook|grr|bar-chart|scatter-chart|time-chart|tool-viewer|velo)-/,
            attributeNameCheck: /value|params|base-url|adtype|caption/,
            allowCustomizedBuiltInElements: true,
        },
    });
    if (DOMPurify.removed.length > 0 ) {
        console.log("Sanitize html in template. Rejected ", DOMPurify.removed);
    }
    return clean;
}

export function cleanupHTML(html) {
    // React expect no whitespace between table elements
    html = html.replace(/>\s*<thead/g, "><thead");
    html = html.replace(/>\s*<tbody/g, "><tbody");
    html = html.replace(/>\s*<tr/g, "><tr");
    html = html.replace(/>\s*<th/g, "><th");
    html = html.replace(/>\s*<td/g, "><td");

    html = html.replace(/>\s*<\/thead/g, "></thead");
    html = html.replace(/>\s*<\/tbody/g, "></tbody");
    html = html.replace(/>\s*<\/tr/g, "></tr");
    html = html.replace(/>\s*<\/th/g, "></th");
    html = html.replace(/>\s*<\/td/g, "></td");

    return html;
};


export default function parseHTML(html, options) {
    html = cleanupHTML(html);
    return parse(sanitize(html), options);
}
