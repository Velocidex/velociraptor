import React from 'react';
import PropTypes from 'prop-types';

import api from '../core/api-service.js';
import VeloTable, { PrepareData } from '../core/table.js';
import _ from 'lodash';

import FormControl from 'react-bootstrap/FormControl';

import Spinner from '../utils/spinner.js';

const MAX_ROWS_PER_TABLE = 500;

function getFlowState(flow) {
    return {flow_id: flow.session_id,
            total_collected_rows: flow.total_collected_rows};
}


export default class FlowResults extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    componentDidMount = () => {
        this.fetchRows();
    }

    // We try really hard to not render the table too often. Tables
    // maintain their own state for e.g. hidden columns and each time
    // we render the table it resets its state.

    // The table needs to render when:
    // 1. the flow id has changed.
    // 2. The flow gained new rows
    // 3. The user selected to view a different artifact result.
    componentDidUpdate(prevProps, prevState, snapshot) {
        if (!_.isEqual(getFlowState(prevProps.flow), getFlowState(this.props.flow)) ||
           prevState.selectedArtifact !== this.state.selectedArtifact) {
            this.fetchRows();
        }
    }

    shouldComponentUpdate(nextProps, nextState) {
        return !_.isEqual(getFlowState(this.props.flow), getFlowState(nextProps.flow)) ||
            !_.isEqual(this.state.pageData, nextState.pageData) ||
            this.state.selectedArtifact !== nextState.selectedArtifact;
    }

    state = {
        selectedArtifact: "",
        loading: true,
        pageData: {},
        total_collected_rows: 0,
    }

    fetchRows = () => {
        let client_id = this.props.flow && this.props.flow.client_id;
        let flow_id = this.props.flow && this.props.flow.session_id;
        let artifacts_with_results = this.props.flow && this.props.flow.artifacts_with_results;

        if (!client_id || !artifacts_with_results || !flow_id) {
            this.setState({selectedArtifact: "", pageData: {},
                           loading: true,
                           total_collected_rows: 0,
                          });
            return;
        }

        let selectedArtifact = this.state.selectedArtifact;
        if (!selectedArtifact || !artifacts_with_results.includes(selectedArtifact)) {
            this.setState({selectedArtifact: artifacts_with_results[0]});
            selectedArtifact = artifacts_with_results[0];
        }

        let params = {
            client_id: client_id,
            flow_id: this.props.flow.session_id,
            artifact: selectedArtifact,
            start_row: 0,
            rows: MAX_ROWS_PER_TABLE,
        };

        this.setState({loading: true,
                       total_collected_rows: this.props.flow.total_collected_rows});

        api.get("v1/GetTable", params).then((response) => {
            this.setState({loading: false,
                           pageData: PrepareData(response.data)});
        }).catch(() => {
            this.setState({loading: false, pageData: {}});
        });
    }

    render() {
        let body = <div>No data available</div>;
        let artifacts_with_results = this.props.flow.artifacts_with_results;

        if (this.state.pageData && this.state.pageData.columns) {
            body = <VeloTable
                     className="col-12"
                     rows={this.state.pageData.rows}
                     columns={this.state.pageData.columns} />;
        }

        return (
            <>
              <Spinner loading={this.state.loading} />
              <FormControl as="select" size="sm"
                           ref={ (el) => this.element=el }
                           onChange={() => {this.setState({
                               selectedArtifact: this.element.value,
                           });}}>
                {_.map(artifacts_with_results, function(item, idx) {
                    return <option key={idx}> {item} </option>;
                })}
              </FormControl>

                  { body }
            </>
        );
    }
};
