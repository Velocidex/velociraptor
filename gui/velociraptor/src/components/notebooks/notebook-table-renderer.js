import React from 'react';
import PropTypes from 'prop-types';

import VeloPagedTable from '../core/paged-table.js';

export default class NotebookTableRenderer extends React.Component {
    static propTypes = {
        refresh: PropTypes.func,
        params: PropTypes.object,
    };

    render() {
        return <VeloPagedTable
                 className="col-12"
                 refresh={this.props.refresh}
                 params={this.props.params}
               />;
    }
};
