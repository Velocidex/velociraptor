import React from 'react';
import PropTypes from 'prop-types';

import api from '../core/api-service.js';
import VeloTable, { PrepareData } from '../core/table.js';
import VeloTimestamp from "../utils/time.js";

import Spinner from '../utils/spinner.js';
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';

const MAX_ROWS_PER_TABLE = 500;

export default class FlowLogs extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    componentDidMount = () => {
        this.fetchRows();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        let prev_flow_id = prevProps.flow && prevProps.flow.session_id;
        let flow_id = this.props.flow && this.props.flow.session_id;
        if (flow_id !== prev_flow_id ||
            prevState.selectedArtifact !== this.state.selectedArtifact) {
            this.fetchRows();
        }
    }

    state = {
        selectedArtifact: "",
        loading: true,
        pageData: {},
    }

    fetchRows = () => {
        let params = {
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
            type: "log",
            start_row: 0,
            rows: MAX_ROWS_PER_TABLE,
        };

        this.setState({loading: true});
        api.get("v1/GetTable", params).then((response) => {
            this.setState({loading: false, pageData: PrepareData(response.data)});
        }).catch(() => {
            this.setState({loading: false, pageData: {}});
        });
    }

    render() {
        if (!this.state.pageData || !this.state.pageData.columns) {
            return (
                <div className="card panel" >
                  <h5 className="card-header">Files uploaded</h5>
                  <div className="card-body">
                    <div>No data available</div>
                  </div>
                </div>
            );
        }

        let renderers = {
            Timestamp: (cell, row, rowIndex) => {
                return (
                    <VeloTimestamp usec={cell / 1000}/>
                );
            },
        };


        return (
            <CardDeck>
              <Card>
                <Card.Header>Client logs</Card.Header>
                <Card.Body>
                  <Spinner loading={this.state.loading}/>
                  <VeloTable
                    className="col-12"
                    renderers={renderers}
                    rows={this.state.pageData.rows}
                    columns={this.state.pageData.columns} />
                </Card.Body>
              </Card>
            </CardDeck>
        );
    }
}
