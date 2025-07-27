import _ from 'lodash';
import PropTypes from 'prop-types';
import React, { Component } from 'react';
import {CancelToken} from 'axios';
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import Button from 'react-bootstrap/Button';
import VeloTable, { getFormatter } from '../core/table.jsx';
import api from '../core/api-service.jsx';
import { parseTableResponse } from '../utils/table.jsx';

export default class TransactionDialog extends Component {
    static propTypes = {
        flow: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    };

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.getDetails();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    state = {
        transactions: [],
    }

    getDetails = ()=>{
        api.get("v1/GetTable", {
            type: "upload_transactions",
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
            rows: 500,
        }, this.source.token).then((response) => {
            let transactions = parseTableResponse(response);

            // Holds all the transactions with a response
            let dedup = {};
            _.each(transactions, row=>{
                if(row.response) {
                    dedup[row.upload_id] = true;
                }
            });

            // Only show the transactions that are still outstanding.
            this.setState({transactions: _.filter(
                transactions, x=>!dedup[x.upload_id])});
        });
    }

    startResumeUploads = ()=>{
        api.post("v1/ResumeFlow", {
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
        }, this.source.token).then((response) => {
            this.props.onClose();
        });
    }

    render() {
        return (
            <Modal show={true}
                   size="lg"
                   dialogClassName="modal-90w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
            <Modal.Title>{T("Resume interrupted uploads")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <VeloTable
                  rows={this.state.transactions}
                  columns={["upload_id", "filename", "accessor",
                            "store_as_name", "expected_size",
                            "start_offset"]}
                  column_renderers={{
                      total_uploaded_bytes: getFormatter("mb"),
                  }}
                  header_renderers={{session_id: "FlowId",
                                     artifacts_with_results: "Artifacts",
                                     total_collected_rows: "Rows",
                                     total_uploaded_bytes: "Bytes",
                                    }}
                />
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary" onClick={this.startResumeUploads}>
                  {T("Resume uploads")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
