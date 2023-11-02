// Safe use of html-react-parser combined it with dompurify to only
// allow proper tag and value.

// NOTE: This is also done in the go server using bluemonday but this
// is a second layer of defence in case something slips in we do it
// again in javascript.
import parse from 'html-react-parser';
import DOMPurify from 'dompurify';

export function sanitize(html) {
    let clean = DOMPurify.sanitize(html, {

        // Line up with NewBlueMondayPolicy() in reporting/gui.go
        CUSTOM_ELEMENT_HANDLING: {
            tagNameCheck: /^(notebook|grr|inline-table-viewer|bar-chart|scatter-chart|time-chart)-/,
            attributeNameCheck: /value|params|base-url/,
            allowCustomizedBuiltInElements: true,
        },
    });
    if (DOMPurify.removed.length > 0 ) {
        console.log("Sanitize html in template. Rejected ", DOMPurify.removed);
    }
    return clean;
}

export default function parseHTML(html, options) {
    return parse(sanitize(html), options);
}
