import React from 'react';
import PropTypes from 'prop-types';

import VeloPagedTable from '../core/paged-table.jsx';
import _ from 'lodash';

import FormControl from 'react-bootstrap/FormControl';

function getFlowState(flow) {
    return {flow_id: flow.session_id,
            active_time: flow.active_time,
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
    // 2. The flow active_time has changed.
    // 3. The user selected to view a different artifact result.
    componentDidUpdate(prevProps, prevState, snapshot) {
        if (!_.isEqual(getFlowState(prevProps.flow), getFlowState(this.props.flow))) {
            this.fetchRows();
        }
    }

    state = {
        params: {},
    }

    setArtifact = (artifact) => {
        let params = Object.assign({}, this.state.params);
        params.artifact = artifact;
        this.setState({params: params});
    }

    fetchRows = () => {
        let client_id = this.props.flow && this.props.flow.client_id;
        let flow_id = this.props.flow && this.props.flow.session_id;
        let artifacts_with_results = this.props.flow && this.props.flow.artifacts_with_results;

        if (!client_id || _.isEmpty(artifacts_with_results) || !flow_id) {
            this.setState({params: {}});
            return;
        }

        let selectedArtifact = this.state.params && this.state.params.artifact;
        if (!selectedArtifact || !artifacts_with_results.includes(selectedArtifact)) {
            selectedArtifact = artifacts_with_results[0];
        }

        let params = {
            client_id: client_id,
            flow_id: this.props.flow.session_id,
            artifact: selectedArtifact,
        };
        this.setState({params: params});
    }

    render() {
        let artifacts_with_results = this.props.flow && this.props.flow.artifacts_with_results;
        if (_.isEmpty(artifacts_with_results)) {
            return <div className="no-content">
                     No Data Available.
                   </div>;
        }
        let flow_id = this.props.flow && this.props.flow.session_id;
        let artifact = this.state.params && this.state.params.artifact;

        return (
            <>
              <FormControl as="select" size="sm"
                           ref={ (el) => this.element=el }
                           onChange={() => this.setArtifact(this.element.value)}>
                {_.map(artifacts_with_results, function(item, idx) {
                    return <option key={idx}> {item} </option>;
                })}
              </FormControl>
              <VeloPagedTable
                env={{
                    client_id: this.props.flow.client_id,
                    flow_id: flow_id,
                }}
                className="col-12"
                name={"FlowResults" + flow_id + artifact}
                params={this.state.params}
                version={getFlowState(this.props.flow)}
              />
            </>
        );
    }
};
