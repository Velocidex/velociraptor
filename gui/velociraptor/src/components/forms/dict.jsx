import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import { formatColumns } from "../core/table.jsx";
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import T from '../i8n/i8n.jsx';
import cellEditFactory, { Type } from 'react-bootstrap-table2-editor';
import BootstrapTable from 'react-bootstrap-table-next';
import filterFactory from 'react-bootstrap-table2-filter';


export default class DictEditor extends Component {
    static propTypes = {
        // This needs to be an array of (k, v) tuples so we can retain
        // ordering.
        value: PropTypes.array,
        setValue: PropTypes.func.isRequired,
    }

    deleteMetadata = k=>{
        let new_obj = _.filter(this.props.value, x=>x[0] !== k);
        this.props.setValue(new_obj);
    }

    setMetadata = (k, v, replace) => {
        let new_obj = [...this.props.value];
        if(replace) {
            new_obj = _.filter(new_obj, x=>x[0] !== replace);
        }

        for(let i=0; i<new_obj.length; i++) {
            // Replace in place.
            if(new_obj[i][0] === k) {
                new_obj[i][1] = v;
                this.props.setValue(new_obj);
                return;
            }
        }

        // Key not found
        new_obj.push([k, v]);
        this.props.setValue(new_obj);
    }

    render() {
        let columns = formatColumns([{
            dataField: "_id",
            text: "",
            style: {
                width: '8%',
            },
            formatter: (cell, row) => {
                return <ButtonGroup>
                    <Button variant="default-outline" size="sm"
                            onClick={() => {
                                // Drop the current row at the
                                // current row index.
                                this.deleteMetadata(row.key);
                            }}
                    >
                      <FontAwesomeIcon icon="trash"/>
                    </Button>
                  </ButtonGroup>;
            },
        }, {
            dataField: "key",
            sort: true,
            editable: true,
            filtered: true,
            classes: "metadata-key",
            text: T("Key"),
        }, {
            dataField: "value",
            editable: true,
            sort: true,
            formatter: this.props.valueRenderer,
            text: T("Value"),
        }]);

        columns[0].headerFormatter = (column, colIndex) => {
            if (colIndex === 0) {
                return <ButtonGroup>
                         <Button variant="default-outline" size="sm"
                                 onClick={() => {
                                     this.setMetadata(
                                         T(" New Key"),
                                         T("New Value"));
                                 }}
                         >
                           <FontAwesomeIcon icon="plus"/>
                         </Button>
                       </ButtonGroup>;
            };
            return column;
        };

        columns[1].editor = {type: Type.TEXTAREA};
        columns[2].editor = {type: Type.TEXTAREA};

        let data = _.map(this.props.value, (x, idx)=>{
            return {_id: idx, key: x[0] || "" , value: x[1] || ""};
        });

        return (
            <BootstrapTable
              hover condensed bootstrap4
              data={data}
              keyField="_id"
              headerClasses="alert alert-secondary"
              columns={columns}
              defaultSorted={[{dataField: "key", order:"asc"}]}
              filter={ filterFactory() }
              cellEdit={ cellEditFactory({
                  mode: 'click',
                  afterSaveCell: (oldValue, newValue, row, column) => {
                      if (oldValue === newValue) return;

                      // We are changing the key value, remove the
                      // old key and store the new value.
                      if (column.dataField == "key" &&
                          oldValue !== row.key) {
                          this.setMetadata(row.key, row.value, oldValue);
                          return;
                      }

                      // We are changing the value
                      this.setMetadata(row.key, newValue);
                  },
                  blurToSave: true,
              }) }
            />
        );
    };
};
