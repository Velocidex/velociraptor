import "./clients-list.css";

import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { withRouter }  from "react-router-dom";

import api from '../core/api-service.js';
import VeloClientStatusIcon from "./client-status.js";
import { formatColumns } from "../core/table.js";
import filterFactory from 'react-bootstrap-table2-filter';
import BootstrapTable from 'react-bootstrap-table-next';
import ClientLink from './client-link.js';
import Spinner from '../utils/spinner.js';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import LabelForm from '../utils/labels.js';
import Row from 'react-bootstrap/Row';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


class LabelClients extends Component {
    static propTypes = {
        affectedClients: PropTypes.array,
        onResolve: PropTypes.func.isRequired,
    }

    labelClients = () => {
        this.props.onResolve();
    }

    state = {
        labels: [],
    }


    render() {
        let clients = this.props.affectedClients || [];
        let columns = formatColumns([
            {dataField: "last_seen_at", text: "Online", sort: true,
             formatter: (cell, row) => {
                 return <VeloClientStatusIcon client={row}/>;
             }},
            {dataField: "client_id", text: "Client ID"},
            {dataField: "os_info.fqdn", text: "Hostname", sort: true},
        ]);

        return (
            <Modal show={true}
                   onHide={this.props.onResolve} >
              <Modal.Header closeButton>
                <Modal.Title>Label Clients</Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <Form.Group as={Row}>
                  <Form.Label column sm="3">Collaborators</Form.Label>
                  <Col sm="8">
                    <LabelForm
                      value={this.state.labels}
                      onChange={(value) => this.setState({labels: value})}/>
                  </Col>
                </Form.Group>
                <BootstrapTable
                    hover
                    condensed
                    keyField="client_id"
                    bootstrap4
                    headerClasses="alert alert-secondary"
                    bodyClasses="fixed-table-body"
                    data={clients}
                    columns={columns}
                />
              </Modal.Body>

              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onResolve}>
                  Close
                </Button>
                <Button variant="primary"
                        onClick={this.labelClients}>
                  Run it!
                </Button>
              </Modal.Footer>
            </Modal>

        );
    }
}

class VeloClientList extends Component {
    static propTypes = {
        query: PropTypes.string.isRequired,
        setClient: PropTypes.func.isRequired,
    }

    state = {
        clients: [],
        selected: [],
        loading: false,
        showLabelDialog: false,
    };

    componentDidMount = () => {
        this.searchClients();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (this.props.query !== prevProps.query) {
            this.searchClients();
        }
    }

    searchClients = () => {
        var query = this.props.query;
        if (!query) {
            return;
        }

        this.setState({loading: true});
        api.get('/api/v1/SearchClients', {
            query: query,
            count: 500,

        }).then(resp => {
            if (resp.data && resp.data.items) {
                this.setState({
                    loading: false,
                    clients: resp.data.items,
                });
            }
            return true;
        });
    }

    openClientInfo = (client) => {
        this.props.setClient(client);

        // Navigate to the host_info page.
        this.props.history.push('/host/' + client.client_id);
    }

    handleOnSelect = (row, isSelect) => {
        if (isSelect) {
            this.setState(() => ({
                selected: [...this.state.selected, row.client_id]
            }));
        } else {
            this.setState(() => ({
                selected: this.state.selected.filter(x => x !== row.client_id)
            }));
        }
    }

    handleOnSelectAll = (isSelect, rows) => {
        const ids = rows.map(r => r.client_id);
        if (isSelect) {
            this.setState(() => ({
                selected: ids
            }));
        } else {
            this.setState(() => ({
                selected: []
            }));
        }
    }

    getAffectedClients = () => {
        let result = [];

        _.each(this.state.selected, (client_id) => {
            _.each(this.state.clients, (client) => {
                if(client.client_id === client_id) {
                    result.push(client);
                };
            });
        });

        return result;
    }

    render() {
        let columns = formatColumns([
            {dataField: "last_seen_at", text: "Online", sort: true,
             formatter: (cell, row) => {
                 return <VeloClientStatusIcon client={row}/>;
             }},
            {dataField: "client_id", text: "Client ID",
             formatter: (cell, row) => {
                 return <ClientLink client_id={cell}/>;
             }},
            {dataField: "os_info.fqdn", text: "Hostname",
             sort: true, filtered: true},
            {dataField: "os_info.release", text: "OS Version"},
            {dataField: "labels", text: "Labels",
             sort:true, filtered: true,
             formatter: (cell, row) => {
                 return _.map(cell, (label, idx) => {
                     return <Button size="sm" key={idx}
                                    onClick={() => {
                                        row.labels = row.labels.filter(x=>x !== label);
                                        this.setState({showLabelDialog: false});
                                    }}
                                    variant="default">
                              {label}
                            </Button>;
                 });
            }},
        ]);


        const selectRow = {
            mode: "checkbox",
            clickToSelect: true,
            classes: "row-selected",
            selected: this.state.selected,
            onSelect: this.handleOnSelect,
            onSelectAll: this.handleOnSelectAll
        };

        return (
            <>
              { this.state.showLabelDialog &&
                <LabelClients
                  affectedClients={this.getAffectedClients()}
                  onResolve={() => {
                      this.setState({showLabelDialog: false});
                      this.searchClients();
                  }}/>}
              <Spinner loading={this.state.loading}/>
              <Navbar className="toolbar">
                <ButtonGroup>
                  <Button title="Label Clients"
                          onClick={() => this.setState({showLabelDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="tags"/>
                  </Button>
                </ButtonGroup>
              </Navbar>
              <div className="fill-parent no-margins toolbar-margin selectable">
                <BootstrapTable
                  hover
                  condensed
                  keyField="client_id"
                  bootstrap4
                  headerClasses="alert alert-secondary"
                  bodyClasses="fixed-table-body"
                  data={this.state.clients}
                  columns={columns}
                  selectRow={ selectRow }
                  filter={ filterFactory() }
                />
              </div>
            </>
        );
    }
};


export default withRouter(VeloClientList);
