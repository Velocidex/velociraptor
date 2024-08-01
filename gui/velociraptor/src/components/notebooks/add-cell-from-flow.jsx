import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';

import { SimpleFlowsList } from '../flows/flows-list.jsx';

import api from '../core/api-service.jsx';
import Modal from 'react-bootstrap/Modal';
import Table from 'react-bootstrap/Table';
import Button from 'react-bootstrap/Button';
import VeloClientSearch from '../clients/search.jsx';
import T from '../i8n/i8n.jsx';

import {CancelToken} from 'axios';


export default class AddCellFromFlowDialog extends React.Component {
    static propTypes = {
        closeDialog: PropTypes.func.isRequired,
        addCell: PropTypes.func,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.setSearch("all");
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    setSearch = (query) => {
        api.get('/v1/SearchClients', {
            query: query,
            limit: 50,
        }, this.source.token).then(resp => {
            let items = resp.data && resp.data.items;
            items = items || [];
            this.setState({loading: false, clients: items});
        });
    }

    addCellFromFlow = (flow) => {
        let client_id = flow.client_id;
        let flow_id = flow.session_id;
        let query = "SELECT * \nFROM source(\n";
        let sources = flow.artifacts_with_results;
        if (!sources) {
            sources = flow.request && flow.request.artifacts;
        }

        if (sources.length<1) {
            return;
        }

        query += "    artifact='" + sources[0] + "',\n";
        for (var i=1; i<sources.length; i++) {
            query += "    -- artifact='" + sources[i] + "',\n";
        }
        query += "    client_id='" + client_id + "',\n    flow_id='" +
            flow_id + "', hunt_id='')\nLIMIT 50\n";

        this.props.addCell(query, "VQL");
        this.props.closeDialog();
    }

    state = {
        loading: false,
        clients: [],
        flows: [],
        selectedClient: null,
        selectedFlow: {},
        query: "all",
    }

    renderClients() {
        return <>
                 <VeloClientSearch setSearch={this.setSearch}/>

                 <Table className="paged-table">
                   <thead>
                     <tr className="paged-table-header">
                       <th>{T("Client ID")}</th>
                       <th>{T("Hostname")}</th>
                       <th>{T("FQDN")}</th>
                       <th>{T("OS Version")}</th>
                     </tr>
                   </thead>
                   <tbody className="fixed-table-body">
                     {_.map(this.state.clients, (c, i)=>{
                         return (
                             <tr key={i}
                                 onClick={()=>this.setState({
                                     selectedClient: c,
                                 })}>
                               <td>{c && c.client_id}</td>
                               <td>{c && c.os_info && c.os_info.hostname}</td>
                               <td>{c && c.os_info && c.os_info.fqdn}</td>
                               <td>{c && c.os_info && c.os_info.release}</td>
                             </tr>);
                     })}
                   </tbody>
                 </Table>

               </>;
    }

    renderFlows() {
        return <>
                 <SimpleFlowsList
                   selected_flow={this.state.selectedFlow}
                   setSelectedFlow={flow=>this.addCellFromFlow(flow)}
                   collapseToggle={()=>""}
                   client_id={this.state.selectedClient.client_id}/>
               </>;
    }

    render() {
        return (
            <Modal show={true}
                   size="lg"
                   className="full-height"
                   dialogClassName="modal-90w"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>{T("Add cell from Flow")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                {!this.state.selectedClient ?
                 this.renderClients() : this.renderFlows()}
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  {T("Cancel")}
                </Button>
                <Button variant="primary"
                        onClick={this.addCellFromHunt}>
                  {T("Submit")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
