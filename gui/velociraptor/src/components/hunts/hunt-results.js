import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import api from '../core/api-service.js';
import VeloTable, { PrepareData } from '../core/table.js';
import FormControl from 'react-bootstrap/FormControl';
import Spinner from '../utils/spinner.js';

const MAX_ROWS_PER_TABLE = 500;

export default class HuntResults extends React.Component {
    static propTypes = {
        hunt: PropTypes.object,
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
        if (prevProps.hunt.hunt_id !== this.props.hunt.hunt_id ||
           prevState.selectedArtifact !== this.state.selectedArtifact) {
            this.fetchRows();
        }
    }

    XXshouldComponentUpdate(nextProps, nextState) {
        return this.props.hunt.hunt_id !== nextProps.hunt.hunt_is ||
            this.state.selectedArtifact !== nextState.selectedArtifact;
    }

    state = {
        selectedArtifact: "",
        loading: false,
        pageData: {},
        total_collected_rows: 0,
    }

    fetchRows = () => {
        let hunt_id = this.props.hunt && this.props.hunt.hunt_id;
        let artifacts_with_results = this.props.hunt && this.props.hunt.artifact_sources;

        if (!hunt_id || !artifacts_with_results) {
            this.setState({selectedArtifact: "", pageData: {},
                           loading: false});
            return;
        }

        let selectedArtifact = this.state.selectedArtifact;
        if (!selectedArtifact || !artifacts_with_results.includes(selectedArtifact)) {
            this.setState({selectedArtifact: artifacts_with_results[0]});
            selectedArtifact = artifacts_with_results[0];
        }

        let params = {
            hunt_id: hunt_id,
            path: hunt_id,
            artifact: selectedArtifact,
            start_row: 0,
            rows: MAX_ROWS_PER_TABLE,
        };

        this.setState({loading: true});
        api.get("v1/GetHuntResults", params).then((response) => {
            this.setState({loading: false,
                           pageData: PrepareData(response.data)});
        }).catch(() => {
            this.setState({loading: false, pageData: {}});
        });
    }

    render() {
        let body = <div>No data available</div>;
        let artifacts_with_results = this.props.hunt.artifact_sources;

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
              <div>
                  { body }
              </div>
            </>
        );
    }
};
