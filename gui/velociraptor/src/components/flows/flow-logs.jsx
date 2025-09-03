import React from 'react';
import PropTypes from 'prop-types';

import VeloPagedTable from '../core/paged-table.jsx';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Form from 'react-bootstrap/Form';
import T from '../i8n/i8n.jsx';

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
        level_column: "level",
    }

    makeTransform() {
        if (this.state.level_filter !== "all") {
            return {
                filter_column: this.state.level_column,
                filter_regex: this.state.level_filter,
            };
        }
        return {};
    }

    getParams = ()=>{
        return {
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
            type: "log",
        };
    }

    getVersion = ()=>{
        return getFlowState(this.props.flow);
    }

    render() {
        let flow_id = this.props.flow && this.props.flow.session_id;
        let formatters = {
            client_time: "timestamp",
            _ts: "timestamp",
            level: "log_level",
            Timestamp: "timestamp",
            message: "log",
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
              formatters={formatters}
              params={this.getParams()}
              translate_column_headers={true}
              toolbar={toolbar}
              version={this.getVersion()}
              transform={this.makeTransform()}
              setTransform={x=>{
                  this.setState({level_filter: "all"});
              }}
              name={"FlowLogs" + flow_id}
            />
        );
    }
}
