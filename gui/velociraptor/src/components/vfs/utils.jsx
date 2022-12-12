/* eslint import/no-anonymous-default-export: [2, {"allowObject": true}] */
export default {
    getSelectedRow: (node) => {
        if (node && node.selected) {
            for(let i=0; i<node.raw_data.length; i++) {
                let row = node.raw_data[i];
                if (row.Name === node.selected) {
                    return row;
                }
            }
        }
        return null;
    }
};
