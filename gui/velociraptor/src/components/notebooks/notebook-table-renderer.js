import React from 'react';
import PropTypes from 'prop-types';

import VeloPagedTable from '../core/paged-table.js';

export default class NotebookTableRenderer extends React.Component {
    static propTypes = {
        params: PropTypes.object,
    };

    render() {
        return <VeloPagedTable
                 className="col-12"
                 params={this.props.params}
               />;
    }
};
