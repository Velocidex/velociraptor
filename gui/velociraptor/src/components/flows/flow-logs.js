import React from 'react';
import PropTypes from 'prop-types';

import VeloPagedTable from '../core/paged-table.js';
import VeloTimestamp from "../utils/time.js";
import LogLevel from '../utils/log_level.js';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Form from 'react-bootstrap/Form';
import T from '../i8n/i8n.js';

function getFlowState(flow) {
    return {flow_id: flow.session_id,
            total_logs: flow.total_logs};
}

export default class FlowLogs extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    state = {
        level_filter: "all",
    }

    makeTransform() {
        if (this.state.level_filter !== "all") {
            return {
                filter_column: "level",
                filter_regex: this.state.level_filter,
            };
        }
        return {};
    }

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
            level:  (cell, row, rowIndex) => {
                return <LogLevel type={cell}/>;
            },
            Timestamp: (cell, row, rowIndex) => {
                return (
                    <VeloTimestamp usec={cell / 1000}/>
                );
            },
        };

        let toolbar = <ButtonGroup className="float-right">
                        <Form.Control
                          as="select"
                          className="flow-log-filter"
                          value={this.state.level_filter}
                          onChange={(e) => {
                              this.setState({level_filter: e.currentTarget.value});
                          }}>
                          <option value="all">{T("Show All")}</option>
                          <option value="debug">{T("Debug")}</option>
                          <option value="info">{T("Info")}</option>
                          <option value="warn">{T("Warn")}</option>
                          <option value="error">{T("Error")}</option>
                        </Form.Control>
                      </ButtonGroup>;

        return (
            <VeloPagedTable
              className="col-12"
              renderers={renderers}
              params={{
                  client_id: this.props.flow.client_id,
                  flow_id: this.props.flow.session_id,
                  type: "log",
              }}
              translate_column_headers={true}
              toolbar={toolbar}
              version={getFlowState(this.props.flow)}
              transform={this.makeTransform()}
              setTransform={x=>{
                  this.setState({level_filter: "all"});
              }}
            />
        );
    }
}
