import _ from 'lodash';

import React from 'react';
import PropTypes from 'prop-types';

import VeloPagedTable from '../core/paged-table.jsx';
import { getFormatter } from '../core/table.jsx';

export default class NotebookTableRenderer extends React.Component {
    static propTypes = {
        env: PropTypes.object,
        refresh: PropTypes.func,
        params: PropTypes.object,
        completion_reporter: PropTypes.func,
        name: PropTypes.string,
    };

    render() {
        let renderers = {};
        _.each(this.props.params.TableOptions, (v, k)=>{
            renderers[k] = getFormatter(v, k);
        });

        return <VeloPagedTable
                 env={this.props.env}
                 className="col-12"
                 renderers={renderers}
                 refresh={this.props.refresh}
                 params={this.props.params}
                 completion_reporter={this.props.completion_reporter}
                 name={this.props.name}
               />;
    }
}
