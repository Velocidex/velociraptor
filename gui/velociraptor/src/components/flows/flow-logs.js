import React from 'react';
import PropTypes from 'prop-types';

import VeloPagedTable from '../core/paged-table.js';
import VeloTimestamp from "../utils/time.js";

export default class FlowLogs extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    render() {
        let renderers = {
            client_time: (cell, row, rowIndex) => {
                return (
                    <VeloTimestamp usec={cell * 1000}/>
                );
            },
            _ts: (cell, row, rowIndex) => {
                return (
                    <VeloTimestamp usec={cell * 1000}/>
                );
            },
            Timestamp: (cell, row, rowIndex) => {
                return (
                    <VeloTimestamp usec={cell / 1000}/>
                );
            },
        };

        return (
            <VeloPagedTable
              className="col-12"
              renderers={renderers}
              params={{
                  client_id: this.props.flow.client_id,
                  flow_id: this.props.flow.session_id,
                  type: "log",
              }}
            />
        );
    }
}
