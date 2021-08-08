import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';

import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import Spinner from '../utils/spinner.js';
import BootstrapTable from 'react-bootstrap-table-next';
import { formatColumns } from "../core/table.js";

import axios from 'axios';
import api from '../core/api-service.js';

export default class AddFlowToHuntDialog extends React.Component {
    static propTypes = {
        client: PropTypes.object,
        flow: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    };

    state = {
        loading: true,
        hunts: [],
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchCompatibleHunts();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    fetchCompatibleHunts = () => {
        console.log(this.props.flow);

        let artifacts = this.props.flow && this.props.flow.request &&
            this.props.flow.request.artifacts;

        api.get("v1/ListHunts", {
            count: 2000,
            offset: 0,
        }, this.source.token).then((response) => {
            if (response.cancel) return;

            let hunts = response.data.items || [];
            let filtered_hunts = _.filter(hunts, x=>{
                return !_.isEmpty(_.intersection(artifacts, x.artifacts));
            });
            this.setState({
                hunts: filtered_hunts,
                loading: false,
            });
        });
    }

    render() {
        let columns = formatColumns([
            {dataField: "hunt_id", text: "HuntId"},
            {dataField: "hunt_description", text: "Description"},
            {dataField: "create_time", text: "Created",
             type: "timestamp", sort: true},
        ]);

        return (
            <Modal show={true}
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>Add collection to hunt</Modal.Title>
              </Modal.Header>
              <Modal.Body><Spinner loading={this.state.loading} />
                You can manually add an existing collection to a running hunt.
                { !_.isEmpty(this.state.hunts) &&
                  <BootstrapTable
                    hover
                    condensed
                    ref={ n => this.node = n }
                    keyField="hunt_id"
                    bootstrap4
                    headerClasses="alert alert-secondary"
                    bodyClasses="fixed-table-body"
                    data={this.state.hunts}
                    columns={columns}
                  />
                }
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  Close
                </Button>
                <Button variant="primary" onClick={this.startDeleteFlow}>
                  Yes do it!
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
};
