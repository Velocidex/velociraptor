import React from 'react';
import PropTypes from 'prop-types';

import VeloTable, { PrepareData } from '../core/table.js';

import Spinner from '../utils/spinner.js';
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';

import api from '../core/api-service.js';

const MAX_ROWS_PER_TABLE = 500;

export default class FlowUploads extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    componentDidMount = () => {
        this.fetchRows();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        let prev_flow_id = prevProps.flow && prevProps.flow.session_id;
        if (this.props.flow.session_id !== prev_flow_id) {
            this.fetchRows();
        }
    }

    state = {
        loading: false,
        pageData: {},
    }

    fetchRows = () => {
        console.log("Fetching rows");
        let params = {
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
            type: "uploads",
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

    downloadFile = (e) => {
        e.stopPropagation();
        e.preventDefault();
    }

    render() {
        if (!this.state.pageData || !this.state.pageData.columns) {
            return (
                <CardDeck>
                  <Card>
                    <Card.Header>Uploaded Files</Card.Header>
                    <Card.Body>
                      <Spinner loading={this.state.loading}/>
                      <div className="no-content">Collection did not upload files</div>
                    </Card.Body>
                  </Card>
                </CardDeck>
            );
        }

        let renderers = {
            // Let users directly download the file without having to
            // make a zip file.
            vfs_path: (cell, row, rowIndex) => {
                return (
                    <a href={api.base_path + "/api/v1/DownloadVFSFile?client_id=" +
                             this.props.flow.client_id +
                             "&vfs_path=" + encodeURIComponent(cell) }>
                      {cell}
                    </a>
                );
            },
        };

        return (
            <CardDeck>
              <Card>
                <Card.Header>Uploaded Files</Card.Header>
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
};
