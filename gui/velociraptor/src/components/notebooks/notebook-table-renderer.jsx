import React from 'react';
import PropTypes from 'prop-types';

import VeloPagedTable from '../core/paged-table.jsx';

export default class NotebookTableRenderer extends React.Component {
    static propTypes = {
        env: PropTypes.object,
        refresh: PropTypes.func,
        params: PropTypes.object,
    };

    render() {
        return <VeloPagedTable
                 env={this.props.env}
                 className="col-12"
                 refresh={this.props.refresh}
                 params={this.props.params}
               />;
    }
};
